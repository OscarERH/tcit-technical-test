package posts

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/oscarruiz/tcit-posts/backend/internal/httpx"
)

const (
	maxAttachmentBytes = 10 << 20
	maxBriefLength     = 20_000
)

// Handler exposes the posts API. Files, Briefs and Workflow are optional
// integrations: a nil value turns the corresponding endpoints into 501s.
type Handler struct {
	Repo     Repository
	Files    FileStore
	Briefs   BriefExtractor
	Workflow Workflow
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/config", httpx.Handler(h.config))
	mux.Handle("GET /api/posts", httpx.Handler(h.list))
	mux.Handle("POST /api/posts", httpx.Handler(h.create))
	mux.Handle("DELETE /api/posts/{id}", httpx.Handler(h.delete))
	mux.Handle("POST /api/posts/{id}/transitions", httpx.Handler(h.transition))
	mux.Handle("POST /api/posts/{id}/attachments", httpx.Handler(h.attach))
	mux.Handle("POST /api/briefs/extract", httpx.Handler(h.extractBrief))
}

// config tells the frontend which optional integrations are available.
func (h *Handler) config(w http.ResponseWriter, r *http.Request) error {
	transitions := []Transition{}
	if h.Workflow != nil {
		var err error
		if transitions, err = h.Workflow.Transitions(r.Context()); err != nil {
			// The core CRUD must keep working without the workflow service.
			slog.Warn("loading workflow definition", "error", err)
			transitions = []Transition{}
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"features": map[string]bool{
			"attachments":     h.Files != nil,
			"briefExtraction": h.Briefs != nil,
			"workflow":        len(transitions) > 0,
		},
		"transitions": transitions,
	})
	return nil
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) error {
	posts, err := h.Repo.List(r.Context())
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, posts)
	return nil
}

type createRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) error {
	var req createRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}

	details := map[string]string{}
	name := validateText(req.Name, "name", NameMaxLength, details)
	description := validateText(req.Description, "description", DescriptionMaxLength, details)
	if len(details) > 0 {
		return &httpx.Error{Status: http.StatusBadRequest, Message: "Invalid post", Details: details}
	}

	post, err := h.Repo.Create(r.Context(), name, description)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusCreated, post)
	return nil
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) error {
	post, err := h.Repo.Delete(r.Context(), r.PathValue("id"))
	if err != nil {
		return mapRepoError(err)
	}

	// The post is already gone; a file left behind in Drive must not fail the request.
	if h.Files != nil {
		for _, a := range post.Attachments {
			if err := h.Files.Delete(r.Context(), a.DriveFileID); err != nil {
				slog.Warn("deleting Drive file", "fileId", a.DriveFileID, "error", err)
			}
		}
	}

	httpx.WriteJSON(w, http.StatusOK, post)
	return nil
}

type transitionRequest struct {
	Transition string `json:"transition"`
}

func (h *Handler) transition(w http.ResponseWriter, r *http.Request) error {
	if h.Workflow == nil {
		return httpx.NewError(http.StatusNotImplemented, "Workflow service is not configured")
	}
	var req transitionRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	if strings.TrimSpace(req.Transition) == "" {
		return httpx.NewError(http.StatusBadRequest, "transition is required")
	}

	id := r.PathValue("id")
	post, err := h.Repo.Get(r.Context(), id)
	if err != nil {
		return mapRepoError(err)
	}
	next, err := h.Workflow.Apply(r.Context(), post.Status, req.Transition)
	if err != nil {
		return err
	}
	post, err = h.Repo.UpdateStatus(r.Context(), id, post.Status, next)
	if err != nil {
		return mapRepoError(err)
	}
	httpx.WriteJSON(w, http.StatusOK, post)
	return nil
}

func (h *Handler) attach(w http.ResponseWriter, r *http.Request) error {
	if h.Files == nil {
		return httpx.NewError(http.StatusNotImplemented, "Google Drive is not configured")
	}

	id := r.PathValue("id")
	if _, err := h.Repo.Get(r.Context(), id); err != nil {
		return mapRepoError(err)
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxAttachmentBytes+1<<20)
	file, header, err := r.FormFile("file")
	if err != nil {
		return httpx.NewError(http.StatusBadRequest,
			fmt.Sprintf("A multipart \"file\" field of at most %d MB is required", maxAttachmentBytes>>20))
	}
	defer file.Close()
	if header.Size > maxAttachmentBytes {
		return httpx.NewError(http.StatusRequestEntityTooLarge, "File is too large")
	}

	fileName := filepath.Base(header.Filename)
	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	fileID, link, err := h.Files.Upload(r.Context(), fileName, mimeType, file)
	if err != nil {
		return fmt.Errorf("uploading to Drive: %w", err)
	}

	attachment, err := h.Repo.AddAttachment(r.Context(), id, Attachment{
		FileName:    fileName,
		MimeType:    mimeType,
		DriveFileID: fileID,
		WebViewLink: link,
	})
	if err != nil {
		// Do not leave an orphan file when the post vanished meanwhile.
		if delErr := h.Files.Delete(r.Context(), fileID); delErr != nil {
			slog.Warn("deleting orphan Drive file", "fileId", fileID, "error", delErr)
		}
		return mapRepoError(err)
	}
	httpx.WriteJSON(w, http.StatusCreated, attachment)
	return nil
}

type extractRequest struct {
	Brief string `json:"brief"`
}

type extractResponse struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// extractBrief suggests post fields from a campaign brief. The brief is sent to
// Gemini and discarded: it is never stored nor written to the logs.
func (h *Handler) extractBrief(w http.ResponseWriter, r *http.Request) error {
	if h.Briefs == nil {
		return httpx.NewError(http.StatusNotImplemented, "Gemini is not configured")
	}
	var req extractRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	brief := strings.TrimSpace(req.Brief)
	if brief == "" {
		return httpx.NewError(http.StatusBadRequest, "brief is required")
	}
	if utf8.RuneCountInString(brief) > maxBriefLength {
		return httpx.NewError(http.StatusBadRequest,
			fmt.Sprintf("brief must be at most %d characters", maxBriefLength))
	}

	name, description, err := h.Briefs.Extract(r.Context(), brief)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, extractResponse{
		Name:        truncate(strings.TrimSpace(name), NameMaxLength),
		Description: truncate(strings.TrimSpace(description), DescriptionMaxLength),
	})
	return nil
}

func validateText(value, field string, maxLength int, details map[string]string) string {
	trimmed := strings.TrimSpace(value)
	switch {
	case trimmed == "":
		details[field] = "is required"
	case utf8.RuneCountInString(trimmed) > maxLength:
		details[field] = fmt.Sprintf("must be at most %d characters", maxLength)
	}
	return trimmed
}

func truncate(s string, maxLength int) string {
	runes := []rune(s)
	if len(runes) <= maxLength {
		return s
	}
	return string(runes[:maxLength])
}

func mapRepoError(err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return httpx.NewError(http.StatusNotFound, "Post not found")
	case errors.Is(err, ErrConflict):
		return httpx.NewError(http.StatusConflict, "Post was modified, try again")
	}
	return err
}

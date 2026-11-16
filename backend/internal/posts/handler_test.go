package posts_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/oscarruiz/tcit-posts/backend/internal/httpx"
	"github.com/oscarruiz/tcit-posts/backend/internal/posts"
)

// fakeRepo is an in-memory stand-in for the Spanner repository.
type fakeRepo struct {
	items      []posts.Post
	next       int
	failAttach error
}

func (f *fakeRepo) List(context.Context) ([]posts.Post, error) { return f.items, nil }

func (f *fakeRepo) Get(_ context.Context, id string) (posts.Post, error) {
	for _, p := range f.items {
		if p.ID == id {
			return p, nil
		}
	}
	return posts.Post{}, posts.ErrNotFound
}

func (f *fakeRepo) Create(_ context.Context, name, description string) (posts.Post, error) {
	f.next++
	p := posts.Post{ID: strconv.Itoa(f.next), Name: name, Description: description,
		Status: posts.StatusDraft, Attachments: []posts.Attachment{}}
	f.items = append(f.items, p)
	return p, nil
}

func (f *fakeRepo) Delete(_ context.Context, id string) (posts.Post, error) {
	for i, p := range f.items {
		if p.ID == id {
			f.items = append(f.items[:i], f.items[i+1:]...)
			return p, nil
		}
	}
	return posts.Post{}, posts.ErrNotFound
}

func (f *fakeRepo) UpdateStatus(_ context.Context, id, from, to string) (posts.Post, error) {
	for i, p := range f.items {
		if p.ID == id {
			if p.Status != from {
				return posts.Post{}, posts.ErrConflict
			}
			f.items[i].Status = to
			return f.items[i], nil
		}
	}
	return posts.Post{}, posts.ErrNotFound
}

func (f *fakeRepo) AddAttachment(_ context.Context, postID string, a posts.Attachment) (posts.Attachment, error) {
	if f.failAttach != nil {
		return posts.Attachment{}, f.failAttach
	}
	for i, p := range f.items {
		if p.ID == postID {
			a.ID = "att-" + strconv.Itoa(len(p.Attachments)+1)
			f.items[i].Attachments = append(f.items[i].Attachments, a)
			return a, nil
		}
	}
	return posts.Attachment{}, posts.ErrNotFound
}

// fakeFiles records what would have been sent to Google Drive.
type fakeFiles struct {
	uploaded map[string]string // file id -> content
	deleted  []string
}

func (f *fakeFiles) Upload(_ context.Context, fileName, _ string, content io.Reader) (string, string, error) {
	data, err := io.ReadAll(content)
	if err != nil {
		return "", "", err
	}
	if f.uploaded == nil {
		f.uploaded = map[string]string{}
	}
	id := "drive-" + fileName
	f.uploaded[id] = string(data)
	return id, "https://drive.example/" + id, nil
}

func (f *fakeFiles) Delete(_ context.Context, fileID string) error {
	f.deleted = append(f.deleted, fileID)
	return nil
}

// upload posts a multipart form with one "file" field.
func upload(t *testing.T, mux *http.ServeMux, path, fileName, content string, out any) int {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", fileName)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(part, content)
	form.Close()

	req := httptest.NewRequest("POST", path, &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if out != nil {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatalf("decoding %q: %v", rec.Body.String(), err)
		}
	}
	return rec.Code
}

type fakeWorkflow struct{}

func (fakeWorkflow) Transitions(context.Context) ([]posts.Transition, error) {
	return []posts.Transition{{Name: "publish", From: []string{"draft"}, To: "published"}}, nil
}

func (fakeWorkflow) Apply(_ context.Context, status, transition string) (string, error) {
	if status == "draft" && transition == "publish" {
		return "published", nil
	}
	return "", httpx.NewError(http.StatusConflict, "not allowed")
}

type fakeBriefs struct{}

func (fakeBriefs) Extract(context.Context, string) (string, string, error) {
	return strings.Repeat("n", 150), " resumen ", nil
}

func newServer(h *posts.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	h.Register(mux)
	return mux
}

func do(t *testing.T, mux *http.ServeMux, method, path, body string, out any) int {
	t.Helper()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	if out != nil {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatalf("decoding %q: %v", rec.Body.String(), err)
		}
	}
	return rec.Code
}

func TestCreateListDelete(t *testing.T) {
	mux := newServer(&posts.Handler{Repo: &fakeRepo{}})

	var created posts.Post
	if code := do(t, mux, "POST", "/api/posts", `{"name":"  Post 1 ","description":"Hola"}`, &created); code != http.StatusCreated {
		t.Fatalf("create status = %d", code)
	}
	if created.Name != "Post 1" || created.Description != "Hola" || created.Status != "draft" {
		t.Fatalf("unexpected created post: %+v", created)
	}

	var list []posts.Post
	if code := do(t, mux, "GET", "/api/posts", "", &list); code != http.StatusOK || len(list) != 1 {
		t.Fatalf("list status = %d, len = %d", code, len(list))
	}

	var deleted posts.Post
	if code := do(t, mux, "DELETE", "/api/posts/"+created.ID, "", &deleted); code != http.StatusOK {
		t.Fatalf("delete status = %d", code)
	}
	if deleted.ID != created.ID || deleted.Name != "Post 1" {
		t.Fatalf("delete must return the deleted post, got %+v", deleted)
	}

	if code := do(t, mux, "DELETE", "/api/posts/"+created.ID, "", nil); code != http.StatusNotFound {
		t.Fatalf("second delete status = %d, want 404", code)
	}
}

func TestCreateValidation(t *testing.T) {
	mux := newServer(&posts.Handler{Repo: &fakeRepo{}})

	var body struct {
		Error struct {
			Details map[string]string `json:"details"`
		} `json:"error"`
	}
	if code := do(t, mux, "POST", "/api/posts", `{"name":" "}`, &body); code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", code)
	}
	if body.Error.Details["name"] == "" || body.Error.Details["description"] == "" {
		t.Fatalf("expected details for both fields, got %v", body.Error.Details)
	}

	if code := do(t, mux, "POST", "/api/posts", `{"name":`, nil); code != http.StatusBadRequest {
		t.Fatalf("malformed JSON status = %d, want 400", code)
	}
}

func TestTransition(t *testing.T) {
	repo := &fakeRepo{}
	mux := newServer(&posts.Handler{Repo: repo, Workflow: fakeWorkflow{}})
	post, _ := repo.Create(context.Background(), "A", "a")

	var updated posts.Post
	path := "/api/posts/" + post.ID + "/transitions"
	if code := do(t, mux, "POST", path, `{"transition":"publish"}`, &updated); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if updated.Status != "published" {
		t.Fatalf("status = %q, want published", updated.Status)
	}
	if code := do(t, mux, "POST", path, `{"transition":"publish"}`, nil); code != http.StatusConflict {
		t.Fatalf("repeated transition status = %d, want 409", code)
	}
}

func TestOptionalIntegrationsDisabled(t *testing.T) {
	mux := newServer(&posts.Handler{Repo: &fakeRepo{}})

	for _, path := range []string{"/api/briefs/extract", "/api/posts/1/transitions", "/api/posts/1/attachments"} {
		if code := do(t, mux, "POST", path, `{}`, nil); code != http.StatusNotImplemented {
			t.Errorf("%s status = %d, want 501", path, code)
		}
	}
}

func TestExtractBriefTruncatesToFieldLimits(t *testing.T) {
	mux := newServer(&posts.Handler{Repo: &fakeRepo{}, Briefs: fakeBriefs{}})

	var out struct{ Name, Description string }
	if code := do(t, mux, "POST", "/api/briefs/extract", `{"brief":"Campaña de verano"}`, &out); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if len(out.Name) != posts.NameMaxLength || out.Description != "resumen" {
		t.Fatalf("unexpected extraction: %d chars, %q", len(out.Name), out.Description)
	}
}

func TestCreateRejectsTooLongFields(t *testing.T) {
	mux := newServer(&posts.Handler{Repo: &fakeRepo{}})

	// Limits count characters, not bytes: 100 accented letters are valid.
	atLimit, _ := json.Marshal(map[string]string{"name": strings.Repeat("ñ", posts.NameMaxLength), "description": "ok"})
	if code := do(t, mux, "POST", "/api/posts", string(atLimit), nil); code != http.StatusCreated {
		t.Fatalf("name at the limit: status = %d, want 201", code)
	}

	tooLong, _ := json.Marshal(map[string]string{
		"name":        strings.Repeat("a", posts.NameMaxLength+1),
		"description": strings.Repeat("b", posts.DescriptionMaxLength+1),
	})
	var body struct {
		Error struct {
			Details map[string]string `json:"details"`
		} `json:"error"`
	}
	if code := do(t, mux, "POST", "/api/posts", string(tooLong), &body); code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", code)
	}
	if len(body.Error.Details) != 2 {
		t.Fatalf("expected both fields rejected, got %v", body.Error.Details)
	}
}

func TestListIsAnEmptyArrayNotNull(t *testing.T) {
	mux := newServer(&posts.Handler{Repo: &fakeRepo{items: []posts.Post{}}})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/posts", nil))
	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Fatalf("body = %s, want []", got)
	}
}

func TestConfigReportsAvailableIntegrations(t *testing.T) {
	type config struct {
		Features    map[string]bool    `json:"features"`
		Transitions []posts.Transition `json:"transitions"`
	}

	var bare config
	do(t, newServer(&posts.Handler{Repo: &fakeRepo{}}), "GET", "/api/config", "", &bare)
	if bare.Features["attachments"] || bare.Features["briefExtraction"] || bare.Features["workflow"] {
		t.Fatalf("no integration should be enabled: %v", bare.Features)
	}
	if bare.Transitions == nil {
		t.Fatal("transitions must be an empty array, not null")
	}

	var full config
	handler := &posts.Handler{Repo: &fakeRepo{}, Files: &fakeFiles{}, Briefs: fakeBriefs{}, Workflow: fakeWorkflow{}}
	do(t, newServer(handler), "GET", "/api/config", "", &full)
	if !full.Features["attachments"] || !full.Features["briefExtraction"] || !full.Features["workflow"] {
		t.Fatalf("every integration should be enabled: %v", full.Features)
	}
	if len(full.Transitions) != 1 || full.Transitions[0].Name != "publish" {
		t.Fatalf("transitions = %+v", full.Transitions)
	}
}

func TestTransitionOnMissingPost(t *testing.T) {
	mux := newServer(&posts.Handler{Repo: &fakeRepo{}, Workflow: fakeWorkflow{}})

	if code := do(t, mux, "POST", "/api/posts/nope/transitions", `{"transition":"publish"}`, nil); code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", code)
	}
	if code := do(t, mux, "POST", "/api/posts/nope/transitions", `{"transition":" "}`, nil); code != http.StatusBadRequest {
		t.Fatalf("blank transition status = %d, want 400", code)
	}
}

func TestAttachUploadsToDriveAndStoresTheReference(t *testing.T) {
	repo, files := &fakeRepo{}, &fakeFiles{}
	mux := newServer(&posts.Handler{Repo: repo, Files: files})
	post, _ := repo.Create(context.Background(), "A", "a")

	var attachment posts.Attachment
	// A path in the file name must not survive: only its base name is kept.
	code := upload(t, mux, "/api/posts/"+post.ID+"/attachments", "../secret/brief.txt", "contenido", &attachment)
	if code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", code)
	}
	if attachment.FileName != "brief.txt" || attachment.DriveFileID != "drive-brief.txt" || attachment.WebViewLink == "" {
		t.Fatalf("unexpected attachment: %+v", attachment)
	}
	if files.uploaded["drive-brief.txt"] != "contenido" {
		t.Fatalf("uploaded = %v", files.uploaded)
	}
	if stored, _ := repo.Get(context.Background(), post.ID); len(stored.Attachments) != 1 {
		t.Fatalf("attachment was not stored: %+v", stored)
	}
}

func TestAttachToMissingPostUploadsNothing(t *testing.T) {
	files := &fakeFiles{}
	mux := newServer(&posts.Handler{Repo: &fakeRepo{}, Files: files})

	if code := upload(t, mux, "/api/posts/nope/attachments", "a.txt", "x", nil); code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", code)
	}
	if len(files.uploaded) != 0 {
		t.Fatalf("nothing should reach Drive, got %v", files.uploaded)
	}
}

func TestAttachWithoutFileField(t *testing.T) {
	repo := &fakeRepo{}
	mux := newServer(&posts.Handler{Repo: repo, Files: &fakeFiles{}})
	post, _ := repo.Create(context.Background(), "A", "a")

	if code := do(t, mux, "POST", "/api/posts/"+post.ID+"/attachments", `{}`, nil); code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", code)
	}
}

func TestAttachRemovesTheDriveFileWhenItCannotBeStored(t *testing.T) {
	repo, files := &fakeRepo{}, &fakeFiles{}
	mux := newServer(&posts.Handler{Repo: repo, Files: files})
	post, _ := repo.Create(context.Background(), "A", "a")
	// The post is deleted between the existence check and the insert.
	repo.failAttach = posts.ErrNotFound

	if code := upload(t, mux, "/api/posts/"+post.ID+"/attachments", "a.txt", "x", nil); code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", code)
	}
	if len(files.deleted) != 1 || files.deleted[0] != "drive-a.txt" {
		t.Fatalf("orphan Drive file was not removed: deleted = %v", files.deleted)
	}
}

func TestDeleteAlsoRemovesDriveFiles(t *testing.T) {
	repo, files := &fakeRepo{}, &fakeFiles{}
	mux := newServer(&posts.Handler{Repo: repo, Files: files})
	post, _ := repo.Create(context.Background(), "A", "a")
	upload(t, mux, "/api/posts/"+post.ID+"/attachments", "uno.txt", "1", nil)
	upload(t, mux, "/api/posts/"+post.ID+"/attachments", "dos.txt", "2", nil)

	if code := do(t, mux, "DELETE", "/api/posts/"+post.ID, "", nil); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if len(files.deleted) != 2 {
		t.Fatalf("deleted Drive files = %v, want both", files.deleted)
	}
}

func TestExtractBriefValidation(t *testing.T) {
	mux := newServer(&posts.Handler{Repo: &fakeRepo{}, Briefs: fakeBriefs{}})

	if code := do(t, mux, "POST", "/api/briefs/extract", `{"brief":"   "}`, nil); code != http.StatusBadRequest {
		t.Fatalf("blank brief status = %d, want 400", code)
	}
	huge, _ := json.Marshal(map[string]string{"brief": strings.Repeat("a", 20_001)})
	if code := do(t, mux, "POST", "/api/briefs/extract", string(huge), nil); code != http.StatusBadRequest {
		t.Fatalf("oversized brief status = %d, want 400", code)
	}
}

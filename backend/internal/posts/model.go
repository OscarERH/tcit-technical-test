// Package posts contains the Post domain, its HTTP API and the contracts the
// API needs from the outside world (storage, Drive, Gemini, workflow engine).
package posts

import (
	"context"
	"errors"
	"io"
	"time"
)

const (
	NameMaxLength        = 100
	DescriptionMaxLength = 500

	// StatusDraft is the initial place of the Symfony "post_publishing" workflow.
	StatusDraft = "draft"
)

var (
	ErrNotFound = errors.New("post not found")
	// ErrConflict means the post changed between reading it and updating it.
	ErrConflict = errors.New("post was modified concurrently")
)

// Post is serialized in camelCase; the Spanner columns are snake_case.
type Post struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Status      string       `json:"status"`
	CreatedAt   time.Time    `json:"createdAt"`
	Attachments []Attachment `json:"attachments"`
}

type Attachment struct {
	ID          string    `json:"id"`
	FileName    string    `json:"fileName"`
	MimeType    string    `json:"mimeType"`
	DriveFileID string    `json:"driveFileId"`
	WebViewLink string    `json:"webViewLink"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Repository interface {
	List(ctx context.Context) ([]Post, error)
	Get(ctx context.Context, id string) (Post, error)
	Create(ctx context.Context, name, description string) (Post, error)
	// Delete removes the post (and its attachment rows) and returns it.
	Delete(ctx context.Context, id string) (Post, error)
	// UpdateStatus moves the post from one status to another, failing with
	// ErrConflict when the stored status is no longer `from`.
	UpdateStatus(ctx context.Context, id, from, to string) (Post, error)
	AddAttachment(ctx context.Context, postID string, a Attachment) (Attachment, error)
}

// FileStore keeps attachment binaries (Google Drive).
type FileStore interface {
	Upload(ctx context.Context, fileName, mimeType string, content io.Reader) (fileID, webViewLink string, err error)
	Delete(ctx context.Context, fileID string) error
}

// BriefExtractor turns a free-text campaign brief into post fields (Gemini).
type BriefExtractor interface {
	Extract(ctx context.Context, brief string) (name, description string, err error)
}

// Transition is one edge of the post lifecycle, as defined by the workflow service.
type Transition struct {
	Name string   `json:"name"`
	From []string `json:"from"`
	To   string   `json:"to"`
}

// Workflow validates status transitions (Symfony Workflow service).
type Workflow interface {
	// Transitions returns the lifecycle definition so clients can offer only
	// the actions that make sense for each post.
	Transitions(ctx context.Context) ([]Transition, error)
	// Apply returns the status reached by applying transition from `status`.
	// It returns an *httpx.Error with status 409 when the transition is not allowed.
	Apply(ctx context.Context, status, transition string) (string, error)
}

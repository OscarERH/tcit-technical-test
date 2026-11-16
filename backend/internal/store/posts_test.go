package store_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"cloud.google.com/go/spanner"

	"github.com/oscarruiz/tcit-posts/backend/internal/posts"
	"github.com/oscarruiz/tcit-posts/backend/internal/store"
)

// newRepository gives each test its own empty database on the Spanner emulator.
// These are integration tests: they are skipped unless SPANNER_EMULATOR_HOST
// points at a running emulator (see the README for the command).
func newRepository(t *testing.T) *store.PostsRepository {
	t.Helper()
	if os.Getenv("SPANNER_EMULATOR_HOST") == "" {
		t.Skip("SPANNER_EMULATOR_HOST is not set; skipping Spanner integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	const project, instance = "tcit-test", "test-instance"
	// Database ids: 2-30 chars, lowercase letters, digits, hyphens and underscores.
	database := fmt.Sprintf("t%d", time.Now().UnixNano()%1e15)
	if err := store.Setup(ctx, project, instance, database); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	// Running it again must be harmless: the API does so on every boot.
	if err := store.Setup(ctx, project, instance, database); err != nil {
		t.Fatalf("second Setup: %v", err)
	}

	client, err := spanner.NewClient(ctx,
		fmt.Sprintf("projects/%s/instances/%s/databases/%s", project, instance, database))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(client.Close)
	return store.NewPostsRepository(client)
}

func TestCreateListAndGet(t *testing.T) {
	repo := newRepository(t)
	ctx := context.Background()

	if list, err := repo.List(ctx); err != nil || list == nil || len(list) != 0 {
		t.Fatalf("empty List = (%v, %v), want an empty non-nil slice", list, err)
	}

	first, err := repo.Create(ctx, "Primero", "uno")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if first.ID == "" || first.Status != posts.StatusDraft || first.CreatedAt.IsZero() || first.Attachments == nil {
		t.Fatalf("unexpected created post: %+v", first)
	}
	second, err := repo.Create(ctx, "Segundo", "dos")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 || list[0].ID != first.ID || list[1].ID != second.ID {
		t.Fatalf("List must return posts in creation order, got %+v", list)
	}
	// The value returned by Create must match what was actually stored.
	if !list[0].CreatedAt.Equal(first.CreatedAt) || list[0].Name != "Primero" || list[0].Description != "uno" {
		t.Fatalf("stored post %+v differs from created post %+v", list[0], first)
	}

	got, err := repo.Get(ctx, second.ID)
	if err != nil || got.Name != "Segundo" {
		t.Fatalf("Get = (%+v, %v)", got, err)
	}
	if _, err := repo.Get(ctx, "missing"); !errors.Is(err, posts.ErrNotFound) {
		t.Fatalf("Get(missing) error = %v, want ErrNotFound", err)
	}
}

func TestDeleteReturnsThePostAndRemovesIt(t *testing.T) {
	repo := newRepository(t)
	ctx := context.Background()
	created, _ := repo.Create(ctx, "Borrar", "x")
	keep, _ := repo.Create(ctx, "Conservar", "y")

	deleted, err := repo.Delete(ctx, created.ID)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if deleted.ID != created.ID || deleted.Name != "Borrar" {
		t.Fatalf("Delete returned %+v", deleted)
	}

	list, _ := repo.List(ctx)
	if len(list) != 1 || list[0].ID != keep.ID {
		t.Fatalf("after delete List = %+v", list)
	}
	if _, err := repo.Delete(ctx, created.ID); !errors.Is(err, posts.ErrNotFound) {
		t.Fatalf("second Delete error = %v, want ErrNotFound", err)
	}
}

func TestUpdateStatus(t *testing.T) {
	repo := newRepository(t)
	ctx := context.Background()
	created, _ := repo.Create(ctx, "Post", "x")

	updated, err := repo.UpdateStatus(ctx, created.ID, "draft", "published")
	if err != nil || updated.Status != "published" || updated.Name != "Post" {
		t.Fatalf("UpdateStatus = (%+v, %v)", updated, err)
	}
	if got, _ := repo.Get(ctx, created.ID); got.Status != "published" {
		t.Fatalf("stored status = %q", got.Status)
	}

	// The post is no longer a draft, so a second "from draft" must not win.
	if _, err := repo.UpdateStatus(ctx, created.ID, "draft", "archived"); !errors.Is(err, posts.ErrConflict) {
		t.Fatalf("stale UpdateStatus error = %v, want ErrConflict", err)
	}
	if got, _ := repo.Get(ctx, created.ID); got.Status != "published" {
		t.Fatalf("status changed despite the conflict: %q", got.Status)
	}
	if _, err := repo.UpdateStatus(ctx, "missing", "draft", "published"); !errors.Is(err, posts.ErrNotFound) {
		t.Fatalf("UpdateStatus(missing) error = %v, want ErrNotFound", err)
	}
}

func TestAttachments(t *testing.T) {
	repo := newRepository(t)
	ctx := context.Background()
	post, _ := repo.Create(ctx, "Con adjunto", "x")
	other, _ := repo.Create(ctx, "Sin adjunto", "y")

	attachment, err := repo.AddAttachment(ctx, post.ID, posts.Attachment{
		FileName: "brief.pdf", MimeType: "application/pdf", DriveFileID: "drive-1", WebViewLink: "https://drive/1",
	})
	if err != nil || attachment.ID == "" || attachment.CreatedAt.IsZero() {
		t.Fatalf("AddAttachment = (%+v, %v)", attachment, err)
	}

	list, _ := repo.List(ctx)
	if len(list[0].Attachments) != 1 || list[0].Attachments[0].DriveFileID != "drive-1" {
		t.Fatalf("List attachments of the post = %+v", list[0].Attachments)
	}
	if list[1].ID != other.ID || len(list[1].Attachments) != 0 {
		t.Fatalf("attachment leaked into another post: %+v", list[1])
	}

	if _, err := repo.AddAttachment(ctx, "missing", posts.Attachment{FileName: "a"}); !errors.Is(err, posts.ErrNotFound) {
		t.Fatalf("AddAttachment(missing post) error = %v, want ErrNotFound", err)
	}

	// Delete hands back the attachments so their Drive files can be removed.
	deleted, err := repo.Delete(ctx, post.ID)
	if err != nil || len(deleted.Attachments) != 1 || deleted.Attachments[0].FileName != "brief.pdf" {
		t.Fatalf("Delete = (%+v, %v)", deleted, err)
	}
}

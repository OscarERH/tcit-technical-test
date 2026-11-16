package store

import (
	"context"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"

	"github.com/oscarruiz/tcit-posts/backend/internal/posts"
)

var (
	postColumns       = []string{"post_id", "name", "description", "status", "created_at"}
	attachmentColumns = []string{"post_id", "attachment_id", "file_name", "mime_type", "drive_file_id", "web_view_link", "created_at"}
)

// reader is satisfied by both read-only and read-write transactions.
type reader interface {
	ReadRow(ctx context.Context, table string, key spanner.Key, columns []string) (*spanner.Row, error)
	Read(ctx context.Context, table string, keys spanner.KeySet, columns []string) *spanner.RowIterator
	Query(ctx context.Context, statement spanner.Statement) *spanner.RowIterator
}

type PostsRepository struct {
	client *spanner.Client
}

func NewPostsRepository(client *spanner.Client) *PostsRepository {
	return &PostsRepository{client: client}
}

func (r *PostsRepository) List(ctx context.Context) ([]posts.Post, error) {
	// A single read-only transaction gives both reads the same snapshot.
	txn := r.client.ReadOnlyTransaction()
	defer txn.Close()

	result := []posts.Post{}
	index := map[string]int{}
	err := txn.Query(ctx, spanner.Statement{
		SQL: `SELECT post_id, name, description, status, created_at FROM posts ORDER BY created_at, post_id`,
	}).Do(func(row *spanner.Row) error {
		post, err := scanPost(row)
		if err != nil {
			return err
		}
		index[post.ID] = len(result)
		result = append(result, post)
		return nil
	})
	if err != nil {
		return nil, err
	}

	err = txn.Query(ctx, spanner.Statement{
		SQL: `SELECT post_id, attachment_id, file_name, mime_type, drive_file_id, web_view_link, created_at
		      FROM post_attachments ORDER BY created_at, attachment_id`,
	}).Do(func(row *spanner.Row) error {
		postID, attachment, err := scanAttachment(row)
		if err != nil {
			return err
		}
		if i, ok := index[postID]; ok {
			result[i].Attachments = append(result[i].Attachments, attachment)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *PostsRepository) Get(ctx context.Context, id string) (posts.Post, error) {
	txn := r.client.ReadOnlyTransaction()
	defer txn.Close()
	return readPost(ctx, txn, id)
}

func (r *PostsRepository) Create(ctx context.Context, name, description string) (posts.Post, error) {
	post := posts.Post{
		ID:          uuid.NewString(),
		Name:        name,
		Description: description,
		Status:      posts.StatusDraft,
		Attachments: []posts.Attachment{},
	}
	commitTime, err := r.client.Apply(ctx, []*spanner.Mutation{
		spanner.Insert("posts", postColumns,
			[]any{post.ID, post.Name, post.Description, post.Status, spanner.CommitTimestamp}),
	})
	if err != nil {
		return posts.Post{}, err
	}
	post.CreatedAt = commitTime
	return post, nil
}

func (r *PostsRepository) Delete(ctx context.Context, id string) (posts.Post, error) {
	var deleted posts.Post
	_, err := r.client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		post, err := readPost(ctx, txn, id)
		if err != nil {
			return err
		}
		deleted = post
		// Attachment rows are interleaved with ON DELETE CASCADE.
		return txn.BufferWrite([]*spanner.Mutation{spanner.Delete("posts", spanner.Key{id})})
	})
	if err != nil {
		return posts.Post{}, err
	}
	return deleted, nil
}

func (r *PostsRepository) UpdateStatus(ctx context.Context, id, from, to string) (posts.Post, error) {
	var updated posts.Post
	_, err := r.client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		post, err := readPost(ctx, txn, id)
		if err != nil {
			return err
		}
		if post.Status != from {
			return posts.ErrConflict
		}
		post.Status = to
		updated = post
		return txn.BufferWrite([]*spanner.Mutation{
			spanner.Update("posts", []string{"post_id", "status"}, []any{id, to}),
		})
	})
	if err != nil {
		return posts.Post{}, err
	}
	return updated, nil
}

func (r *PostsRepository) AddAttachment(ctx context.Context, postID string, a posts.Attachment) (posts.Attachment, error) {
	a.ID = uuid.NewString()
	commitTime, err := r.client.Apply(ctx, []*spanner.Mutation{
		spanner.Insert("post_attachments", attachmentColumns,
			[]any{postID, a.ID, a.FileName, a.MimeType, a.DriveFileID, a.WebViewLink, spanner.CommitTimestamp}),
	})
	if err != nil {
		// Inserting a child row without its interleaved parent fails with NotFound.
		if spanner.ErrCode(err) == codes.NotFound {
			return posts.Attachment{}, posts.ErrNotFound
		}
		return posts.Attachment{}, err
	}
	a.CreatedAt = commitTime
	return a, nil
}

func readPost(ctx context.Context, txn reader, id string) (posts.Post, error) {
	row, err := txn.ReadRow(ctx, "posts", spanner.Key{id}, postColumns)
	if err != nil {
		if spanner.ErrCode(err) == codes.NotFound {
			return posts.Post{}, posts.ErrNotFound
		}
		return posts.Post{}, err
	}
	post, err := scanPost(row)
	if err != nil {
		return posts.Post{}, err
	}

	// Attachments share the post's key prefix, so this is a local range read.
	err = txn.Read(ctx, "post_attachments", spanner.Key{id}.AsPrefix(), attachmentColumns).
		Do(func(row *spanner.Row) error {
			_, attachment, err := scanAttachment(row)
			if err != nil {
				return err
			}
			post.Attachments = append(post.Attachments, attachment)
			return nil
		})
	if err != nil {
		return posts.Post{}, err
	}
	return post, nil
}

func scanPost(row *spanner.Row) (posts.Post, error) {
	var (
		post      posts.Post
		createdAt time.Time
	)
	if err := row.Columns(&post.ID, &post.Name, &post.Description, &post.Status, &createdAt); err != nil {
		return posts.Post{}, err
	}
	post.CreatedAt = createdAt
	post.Attachments = []posts.Attachment{}
	return post, nil
}

func scanAttachment(row *spanner.Row) (string, posts.Attachment, error) {
	var (
		postID string
		a      posts.Attachment
	)
	err := row.Columns(&postID, &a.ID, &a.FileName, &a.MimeType, &a.DriveFileID, &a.WebViewLink, &a.CreatedAt)
	return postID, a, err
}

// Package drive stores post attachments in Google Drive.
package drive

import (
	"context"
	"fmt"
	"io"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

type Store struct {
	service  *drive.Service
	folderID string
}

// NewStore authenticates with a service account key file. Service accounts have
// no storage quota of their own, so folderID must be a folder inside a shared
// drive where the service account is a member.
func NewStore(ctx context.Context, credentialsFile, folderID string) (*Store, error) {
	service, err := drive.NewService(ctx,
		option.WithCredentialsFile(credentialsFile),
		option.WithScopes(drive.DriveScope),
	)
	if err != nil {
		return nil, fmt.Errorf("creating Drive service: %w", err)
	}
	return &Store{service: service, folderID: folderID}, nil
}

func (s *Store) Upload(ctx context.Context, fileName, mimeType string, content io.Reader) (string, string, error) {
	file, err := s.service.Files.
		Create(&drive.File{Name: fileName, Parents: []string{s.folderID}}).
		Media(content, googleapi.ContentType(mimeType)).
		SupportsAllDrives(true).
		Fields("id, webViewLink").
		Context(ctx).
		Do()
	if err != nil {
		return "", "", err
	}
	return file.Id, file.WebViewLink, nil
}

func (s *Store) Delete(ctx context.Context, fileID string) error {
	return s.service.Files.Delete(fileID).SupportsAllDrives(true).Context(ctx).Do()
}

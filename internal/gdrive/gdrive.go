// Package gdrive provides a Google Drive client for listing and downloading
// training videos stored in a specific Drive folder. The video list is cached
// in memory to avoid repeated API calls.
package gdrive

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

// VideoItem describes a single training video stored in Google Drive.
type VideoItem struct {
	// ID is the Google Drive file identifier used for downloads.
	ID string
	// Name is the human-readable filename shown to users.
	Name string
	// WebContentLink is a direct download URL that works when the file is
	// publicly shared ("anyone with the link"). Used by Instagram/WhatsApp
	// which require a public URL instead of a streamed upload.
	WebContentLink string
}

// DriveService lists and downloads training videos from a Google Drive folder.
type DriveService interface {
	// ListVideos returns all video files from the configured folder.
	// Results are cached for the TTL specified at construction.
	ListVideos() ([]VideoItem, error)
	// DownloadVideo opens a streaming download for the given file ID.
	// The caller must close the returned ReadCloser.
	DownloadVideo(id string) (io.ReadCloser, error)
}

type driveService struct {
	svc      *drive.Service
	folderID string
	ttl      time.Duration

	mu          sync.RWMutex
	cache       []VideoItem
	lastUpdated time.Time
}

// NewDriveService creates a DriveService authenticated via a service-account
// JSON credentials file. The video list is refreshed at most once per ttl.
// Share the Drive folder with the service account e-mail to grant access.
func NewDriveService(credentialsFile, folderID string, ttl time.Duration) (DriveService, error) {
	data, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("gdrive: read credentials: %w", err)
	}

	creds, err := google.CredentialsFromJSON(context.Background(), data, drive.DriveReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("gdrive: parse credentials: %w", err)
	}

	svc, err := drive.NewService(context.Background(), option.WithCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("gdrive: create service: %w", err)
	}

	return &driveService{svc: svc, folderID: folderID, ttl: ttl}, nil
}

// ListVideos returns the cached video list, refreshing it from Drive if the
// cache has expired.
func (d *driveService) ListVideos() ([]VideoItem, error) {
	d.mu.RLock()
	if d.cache != nil && time.Since(d.lastUpdated) < d.ttl {
		videos := d.cache
		d.mu.RUnlock()
		return videos, nil
	}
	d.mu.RUnlock()

	q := fmt.Sprintf("'%s' in parents and mimeType contains 'video/' and trashed = false", d.folderID)
	result, err := d.svc.Files.List().
		Q(q).
		Fields("files(id,name,webContentLink)").
		OrderBy("name").
		Do()
	if err != nil {
		return nil, fmt.Errorf("gdrive: list videos: %w", err)
	}

	videos := make([]VideoItem, 0, len(result.Files))
	for _, f := range result.Files {
		videos = append(videos, VideoItem{
			ID:             f.Id,
			Name:           f.Name,
			WebContentLink: f.WebContentLink,
		})
	}

	d.mu.Lock()
	d.cache = videos
	d.lastUpdated = time.Now()
	d.mu.Unlock()

	return videos, nil
}

// DownloadVideo opens a streaming HTTP download for the specified Drive file.
// AcknowledgeAbuse is set to allow downloading files flagged by Drive's
// abuse detection (required for some video formats).
func (d *driveService) DownloadVideo(id string) (io.ReadCloser, error) {
	resp, err := d.svc.Files.Get(id).AcknowledgeAbuse(true).Download()
	if err != nil {
		return nil, fmt.Errorf("gdrive: download %s: %w", id, err)
	}
	return resp.Body, nil
}

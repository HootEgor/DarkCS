// Package gdrive provides a Google Drive client for listing and downloading
// training videos stored in a specific Drive folder. The video list is cached
// in memory to avoid repeated API calls.
//
// Two credential types are supported:
//   - Service account JSON ("type": "service_account") — preferred for servers.
//   - OAuth2 installed-app credentials ("installed" key) combined with a saved
//     token file (token.json). The token file is generated once via the
//     cmd/gdrive-auth helper and contains the refresh token.
package gdrive

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"golang.org/x/oauth2"
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
	log      *slog.Logger

	mu          sync.RWMutex
	cache       []VideoItem
	lastUpdated time.Time
}

// httpTimeout caps every outbound HTTP request to googleapis.com, including
// OAuth2 token exchanges and Drive API calls. Without an explicit dial/read
// timeout the bot goroutine can hang indefinitely on a stalled connection.
const httpTimeout = 20 * time.Second

// timedHTTPClient builds an *http.Client with explicit dial, TLS, and
// response-header timeouts. Using only http.Client.Timeout is not enough —
// it does not fire when a TCP connection stalls before sending any bytes.
func timedHTTPClient() *http.Client {
	t := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
	}
	return &http.Client{Transport: t, Timeout: httpTimeout}
}

// credentialType peeks at the JSON to distinguish service-account credentials
// from OAuth2 installed-app credentials without a full parse.
func credentialType(data []byte) string {
	var probe struct {
		Type      string `json:"type"`
		Installed any    `json:"installed"`
		Web       any    `json:"web"`
	}
	_ = json.Unmarshal(data, &probe)
	if probe.Type != "" {
		return probe.Type // e.g. "service_account"
	}
	if probe.Installed != nil {
		return "installed"
	}
	if probe.Web != nil {
		return "web"
	}
	return "unknown"
}

// NewDriveService creates a DriveService from the given credentials file.
//
// If credentialsFile contains a service-account key, tokenFile is ignored.
// If credentialsFile contains OAuth2 installed-app credentials, tokenFile must
// point to a saved token file (generated once with cmd/gdrive-auth).
//
// The video list is refreshed at most once per ttl. Share the Drive folder
// with the service account email (or the Google account used in the OAuth
// flow) to grant read access.
func NewDriveService(credentialsFile, tokenFile, folderID string, ttl time.Duration, log *slog.Logger) (DriveService, error) {
	data, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("gdrive: read credentials: %w", err)
	}

	baseHTTP := timedHTTPClient()
	// oauth2.HTTPClient in context makes the oauth2 library use baseHTTP for
	// all token fetch requests (JWT exchanges, refresh-token calls, etc.).
	authCtx := context.WithValue(context.Background(), oauth2.HTTPClient, baseHTTP)

	ct := credentialType(data)
	log.Info("gdrive: credential type detected", slog.String("type", ct))

	var tokenSource oauth2.TokenSource

	switch ct {
	case "service_account":
		creds, err := google.CredentialsFromJSON(authCtx, data, drive.DriveReadonlyScope)
		if err != nil {
			return nil, fmt.Errorf("gdrive: parse service-account credentials: %w", err)
		}
		tokenSource = creds.TokenSource

	case "installed", "web":
		// OAuth2 installed/web-app flow: requires a pre-saved token file.
		// Generate it once with cmd/gdrive-auth, then copy to the server.
		if tokenFile == "" {
			return nil, fmt.Errorf("gdrive: token_file is required for OAuth2 installed-app credentials")
		}
		cfg, err := google.ConfigFromJSON(data, drive.DriveReadonlyScope)
		if err != nil {
			return nil, fmt.Errorf("gdrive: parse OAuth2 config: %w", err)
		}
		tokData, err := os.ReadFile(tokenFile)
		if err != nil {
			return nil, fmt.Errorf("gdrive: read token file: %w", err)
		}
		var tok oauth2.Token
		if err := json.Unmarshal(tokData, &tok); err != nil {
			return nil, fmt.Errorf("gdrive: parse token file: %w", err)
		}
		// cfg.TokenSource auto-refreshes using the refresh_token when the
		// access_token expires. Refreshed tokens are kept in memory only;
		// the file is not updated, but refresh_tokens don't expire unless
		// explicitly revoked.
		tokenSource = cfg.TokenSource(authCtx, &tok)

	default:
		return nil, fmt.Errorf("gdrive: unsupported credential type %q", ct)
	}

	// Build the OAuth2 HTTP client from the token source explicitly so that
	// Drive API calls (not just token fetches) travel through timedTransport.
	oauthClient := oauth2.NewClient(authCtx, tokenSource)
	oauthClient.Timeout = httpTimeout

	svc, err := drive.NewService(context.Background(), option.WithHTTPClient(oauthClient))
	if err != nil {
		return nil, fmt.Errorf("gdrive: create service: %w", err)
	}

	return &driveService{svc: svc, folderID: folderID, ttl: ttl, log: log}, nil
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

	d.log.Debug("gdrive: starting Drive API call")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	q := fmt.Sprintf("'%s' in parents and mimeType contains 'video/' and trashed = false", d.folderID)
	d.log.Debug("gdrive: calling Files.List", slog.String("folder_id", d.folderID))
	result, err := d.svc.Files.List().
		Q(q).
		Fields("files(id,name,webContentLink)").
		OrderBy("name").
		Context(ctx).
		Do()
	d.log.Debug("gdrive: Files.List returned", slog.Bool("error", err != nil))
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

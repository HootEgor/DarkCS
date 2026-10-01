package insta

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"sync"
	"time"

	"DarkCS/bot/chat"
	igmessenger "DarkCS/bot/chat/instagram"
	"DarkCS/entity"
	"DarkCS/internal/lib/dedup"
	"DarkCS/internal/lib/safego"
	"DarkCS/internal/lib/sl"
)

const graphAPIURL = "https://graph.instagram.com/v24.0/me/messages"

// tokenRefreshURL is the Instagram endpoint for refreshing long-lived access tokens.
const tokenRefreshURL = "https://graph.instagram.com/refresh_access_token"

// tokenRefreshInterval controls how often the token is proactively renewed.
// Instagram long-lived tokens expire after 60 days; 30 days gives a safe buffer.
const tokenRefreshInterval = 30 * 24 * time.Hour

// InstaBot handles Instagram messaging via the Graph API
type InstaBot struct {
	log            *slog.Logger
	mu             sync.RWMutex
	accessToken    string
	verifyToken    string
	appSecret      string
	fallbackToken  string     // token from config, see SetFallbackToken
	seen           *dedup.Set // message IDs already processed (Meta redelivers)
	chatEngine     *chat.ChatEngine
	tokenPersister func(token string) error // nil = no persistence
}

// SetTokenPersister registers a callback that is called after every successful token refresh.
// Use this to persist the refreshed token to a database so it survives restarts.
func (b *InstaBot) SetTokenPersister(fn func(string) error) {
	b.tokenPersister = fn
}

// WebhookPayload represents the incoming webhook payload from Instagram
type WebhookPayload struct {
	Object string `json:"object"`
	Entry  []struct {
		ID        string `json:"id"`
		Time      int64  `json:"time"`
		Messaging []struct {
			Sender struct {
				ID string `json:"id"`
			} `json:"sender"`
			Recipient struct {
				ID string `json:"id"`
			} `json:"recipient"`
			Timestamp int64 `json:"timestamp"`
			Message   *struct {
				Mid         string `json:"mid"`
				Text        string `json:"text"`
				IsEcho      bool   `json:"is_echo,omitempty"`
				Attachments []struct {
					Type    string `json:"type"`
					Payload struct {
						URL string `json:"url"`
					} `json:"payload"`
				} `json:"attachments,omitempty"`
			} `json:"message,omitempty"`
		} `json:"messaging"`
	} `json:"entry"`
}

// SendMessageRequest represents the request body for sending a message
type SendMessageRequest struct {
	Recipient struct {
		ID string `json:"id"`
	} `json:"recipient"`
	Message struct {
		Text string `json:"text"`
	} `json:"message"`
}

// maxWebhookBody caps an incoming webhook request body.
const maxWebhookBody = 5 << 20

// NewInstaBot creates a new Instagram bot instance
func NewInstaBot(accessToken, verifyToken, appSecret string, log *slog.Logger) *InstaBot {
	b := &InstaBot{
		log:         log.With(sl.Module("instabot")),
		accessToken: accessToken,
		verifyToken: verifyToken,
		appSecret:   appSecret,
		seen:        dedup.New(10 * time.Minute),
	}
	if appSecret == "" {
		b.log.Error("instagram app_secret is empty: webhook signatures are NOT verified, anyone can post fake messages")
	}
	return b
}

// SetChatEngine sets the unified chat engine for this bot.
func (b *InstaBot) SetChatEngine(engine *chat.ChatEngine) {
	b.chatEngine = engine
}

// token returns the current access token under a read lock.
func (b *InstaBot) token() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.accessToken
}

// refreshToken exchanges the current long-lived token for a new one.
// Instagram long-lived tokens expire after 60 days and must be refreshed before expiry.
func (b *InstaBot) refreshToken() error {
	reqURL := fmt.Sprintf("%s?grant_type=ig_refresh_token&access_token=%s", tokenRefreshURL, b.token())
	resp, err := graphClient.Get(reqURL)
	if err != nil {
		return fmt.Errorf("refresh request failed: %w", sl.RedactURLError(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("refresh API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode refresh response: %w", err)
	}
	if result.AccessToken == "" {
		return fmt.Errorf("empty access token in refresh response")
	}

	b.mu.Lock()
	b.accessToken = result.AccessToken
	b.mu.Unlock()

	b.log.Info("access token refreshed", slog.Int("expires_in_seconds", result.ExpiresIn))

	if b.tokenPersister != nil {
		if err := b.tokenPersister(result.AccessToken); err != nil {
			b.log.Error("failed to persist refreshed Instagram token", slog.String("error", err.Error()))
		}
	}
	return nil
}

// Retry schedule for failed refreshes: a single transient error must not leave the token
// unrefreshed for a whole tokenRefreshInterval, or it can expire in the meantime.
const (
	tokenRetryMin = time.Hour
	tokenRetryMax = 24 * time.Hour
)

// SetFallbackToken registers the token from config. It is tried when refreshing the
// current (persisted) token fails, which covers an operator replacing an expired token
// in config while an older one is still stored in MongoDB.
func (b *InstaBot) SetFallbackToken(token string) {
	b.mu.Lock()
	b.fallbackToken = token
	b.mu.Unlock()
}

// refreshWithFallback refreshes the current token and, if that fails, retries once with
// the fallback token.
func (b *InstaBot) refreshWithFallback() error {
	err := b.refreshToken()
	if err == nil {
		return nil
	}

	b.mu.Lock()
	previous := b.accessToken
	fallback := b.fallbackToken
	switched := fallback != "" && fallback != previous
	if switched {
		b.accessToken = fallback
	}
	b.mu.Unlock()
	if !switched {
		return err
	}

	b.log.Warn("refreshing persisted instagram token failed, trying config token", sl.Err(err))
	if fbErr := b.refreshToken(); fbErr != nil {
		// Both failed; keep the persisted token, since the first failure may be transient.
		b.mu.Lock()
		b.accessToken = previous
		b.mu.Unlock()
		return fmt.Errorf("persisted token: %w; config token: %v", err, fbErr)
	}
	return nil
}

// StartTokenRefresh spawns a background goroutine that refreshes the long-lived Instagram
// access token immediately on startup and then every tokenRefreshInterval, retrying
// failures with exponential backoff. It stops when ctx is cancelled. The immediate refresh
// ensures the persisted token is always long-lived so other services (e.g. DarkBot) can
// import it right after startup.
func (b *InstaBot) StartTokenRefresh(ctx context.Context) {
	safego.Go(b.log, "instagram token refresh", func() {
		retry := tokenRetryMin
		for {
			wait := tokenRefreshInterval
			if err := b.refreshWithFallback(); err != nil {
				b.log.Error("failed to refresh Instagram access token",
					slog.Duration("retry_in", retry), sl.Err(err))
				wait = retry
				retry = min(retry*2, tokenRetryMax)
			} else {
				retry = tokenRetryMin
			}

			timer := time.NewTimer(wait)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return
			}
		}
	})
}

// HandleWebhookVerification handles the GET request for webhook verification
func (b *InstaBot) HandleWebhookVerification(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("hub.mode")
	token := r.URL.Query().Get("hub.verify_token")
	challenge := r.URL.Query().Get("hub.challenge")

	// An empty configured token must never match an empty hub.verify_token.
	tokenMatch := b.verifyToken != "" &&
		subtle.ConstantTimeCompare([]byte(token), []byte(b.verifyToken)) == 1
	if mode == "subscribe" && tokenMatch {
		b.log.Info("webhook verified")
		// The challenge is echoed back; plain text + nosniff stops it rendering as HTML.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(challenge))
		return
	}

	b.log.Warn("webhook verification failed",
		slog.String("mode", mode),
		slog.Bool("token_match", tokenMatch),
	)
	http.Error(w, "Forbidden", http.StatusForbidden)
}

// HandleWebhook handles incoming webhook POST requests
func (b *InstaBot) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	// Bounded read: this endpoint is unauthenticated and the body is read before the
	// signature can be checked. Meta webhook payloads are a few KB.
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBody))
	if err != nil {
		b.log.Error("failed to read request body", sl.Err(err))
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Always respond with 200 OK to acknowledge receipt (Meta requires quick response)
	w.WriteHeader(http.StatusOK)

	// Verify signature if app secret is configured
	sigValid := true
	if b.appSecret != "" {
		signature := r.Header.Get("X-Hub-Signature-256")
		if !b.verifySignature(body, signature) {
			b.log.Warn("invalid webhook signature",
				slog.Int("body_bytes", len(body)), slog.String("remote_addr", r.RemoteAddr))
			sigValid = false
		}
	}

	var payload WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		b.log.Error("failed to parse webhook payload", sl.Err(err))
		return
	}

	// Only process messages with valid signatures
	if !sigValid {
		return
	}

	// Process messages asynchronously
	safego.Go(b.log, "instagram webhook", func() { b.processPayload(payload) })
}

// processPayload processes the webhook payload
func (b *InstaBot) processPayload(payload WebhookPayload) {
	if payload.Object != "instagram" {
		return
	}

	for _, entry := range payload.Entry {
		for _, messaging := range entry.Messaging {
			if messaging.Message == nil || messaging.Message.IsEcho {
				continue
			}
			if !b.seen.FirstSeen(messaging.Message.Mid) {
				b.log.Debug("skipping redelivered message", slog.String("mid", messaging.Message.Mid))
				continue
			}

			senderID := messaging.Sender.ID
			text := messaging.Message.Text

			// Handle attachments (photos, files, etc.)
			if b.chatEngine != nil && len(messaging.Message.Attachments) > 0 {
				if listener := b.chatEngine.GetMessageListener(); listener != nil {
					for _, att := range messaging.Message.Attachments {
						if att.Payload.URL == "" {
							continue
						}
						b.downloadAndUploadAttachment(listener, senderID, att.Payload.URL, att.Type, text)
						// Caption only with first attachment
						text = ""
					}

					if username, err := b.GetUserUsername(senderID); err == nil && username != "" {
						listener.UpdateUserPlatformInfo("instagram", senderID, "@"+username)
					}
				}
				continue
			}

			if text == "" {
				continue
			}

			// Save incoming message for CRM
			if b.chatEngine != nil {
				if listener := b.chatEngine.GetMessageListener(); listener != nil {
					listener.SaveAndBroadcastChatMessage(entity.ChatMessage{
						Platform:  "instagram",
						UserID:    senderID,
						ChatID:    senderID,
						Direction: "incoming",
						Sender:    "user",
						Text:      text,
						CreatedAt: time.Now(),
					})

					// Fetch and save Instagram @username
					if username, err := b.GetUserUsername(senderID); err == nil && username != "" {
						listener.UpdateUserPlatformInfo("instagram", senderID, "@"+username)
					}
				}
			}

			// Delegate to ChatEngine if available
			if b.chatEngine != nil {
				messenger := igmessenger.NewMessenger(b)
				if err := b.chatEngine.HandleMessage(context.Background(), messenger, "instagram", senderID, senderID, text); err != nil {
					b.log.Error("chat engine error",
						slog.String("sender_id", senderID),
						sl.Err(err),
					)
				}
				continue
			}

			// Fallback: echo
			echoText := fmt.Sprintf("Echo: %s", text)
			if err := b.SendMessage(senderID, echoText); err != nil {
				b.log.Error("failed to send echo message",
					slog.String("sender_id", senderID),
					sl.Err(err),
				)
			}
		}
	}
}

// SendMessage sends a text message to the specified recipient
func (b *InstaBot) SendMessage(recipientID, text string) error {
	reqBody := SendMessageRequest{}
	reqBody.Recipient.ID = recipientID
	reqBody.Message.Text = text

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s?access_token=%s", graphAPIURL, b.token())
	resp, err := graphClient.Post(url, "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to send request: %w", sl.RedactURLError(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	b.log.Info("message sent successfully", slog.String("recipient_id", recipientID))
	return nil
}

// GetUserUsername fetches the Instagram username for a given user ID via Graph API.
func (b *InstaBot) GetUserUsername(userID string) (string, error) {
	url := fmt.Sprintf("https://graph.instagram.com/v24.0/%s?fields=username&access_token=%s", userID, b.token())
	resp, err := graphClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch user profile: %w", sl.RedactURLError(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error (status %d)", resp.StatusCode)
	}

	var result struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Username, nil
}

// downloadAndUploadAttachment downloads a file from a URL and uploads it to GridFS via the listener.
func (b *InstaBot) downloadAndUploadAttachment(listener chat.MessageListener, senderID, fileURL, attType, caption string) {
	resp, err := mediaClient.Get(fileURL)
	if err != nil {
		b.log.Error("failed to download Instagram attachment",
			slog.String("sender_id", senderID),
			slog.String("url", fileURL),
			sl.Err(err),
		)
		return
	}
	defer resp.Body.Close()

	// Determine filename and MIME type
	mimeType := resp.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	filename := "attachment"
	if parsed, err := url.Parse(fileURL); err == nil {
		base := path.Base(parsed.Path)
		if base != "" && base != "." && base != "/" {
			filename = base
		}
	}

	if err := listener.UploadAndSaveFile("instagram", senderID, resp.Body, filename, mimeType, resp.ContentLength, caption); err != nil {
		b.log.Error("failed to upload Instagram attachment",
			slog.String("sender_id", senderID),
			sl.Err(err),
		)
		if errors.Is(err, entity.ErrFileTooLarge) {
			limitMB := entity.MaxFileSize >> 20
			text := fmt.Sprintf("Файл занадто великий. Максимально дозволений розмір - %d MB.", limitMB)
			_ = b.SendMessage(senderID, text)
		}
	}
}

// SendMediaMessage sends a media attachment to a recipient via Instagram Graph API.
func (b *InstaBot) SendMediaMessage(recipientID, mediaURL, mediaType string) error {
	payload := map[string]interface{}{
		"recipient": map[string]string{"id": recipientID},
		"message": map[string]interface{}{
			"attachment": map[string]interface{}{
				"type": mediaType,
				"payload": map[string]string{
					"url": mediaURL,
				},
			},
		},
	}

	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal media request: %w", err)
	}

	apiURL := fmt.Sprintf("%s?access_token=%s", graphAPIURL, b.token())
	resp, err := graphClient.Post(apiURL, "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to send media message: %w", sl.RedactURLError(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// verifySignature verifies the X-Hub-Signature-256 header
func (b *InstaBot) verifySignature(body []byte, signature string) bool {
	if signature == "" {
		return false
	}

	// Signature format: "sha256=<hex_signature>"
	if len(signature) < 8 || signature[:7] != "sha256=" {
		return false
	}

	expectedSig := signature[7:]
	mac := hmac.New(sha256.New, []byte(b.appSecret))
	mac.Write(body)
	actualSig := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expectedSig), []byte(actualSig))
}

// HTTP clients with timeouts: the default client has none, so a stalled Graph API or CDN
// connection would hang the webhook goroutine forever.
var (
	graphClient = &http.Client{Timeout: 30 * time.Second}
	mediaClient = &http.Client{Timeout: 2 * time.Minute}
)

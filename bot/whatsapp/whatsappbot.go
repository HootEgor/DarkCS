package whatsapp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"DarkCS/bot/chat"
	wamessenger "DarkCS/bot/chat/whatsapp"
	"DarkCS/entity"
	"DarkCS/internal/lib/sl"
)

const graphAPIURL = "https://graph.facebook.com/v21.0"

// WhatsAppBot handles WhatsApp messaging via the Graph API
type WhatsAppBot struct {
	log           *slog.Logger
	accessToken   string
	verifyToken   string
	appSecret     string
	phoneNumberID string
	chatEngine    *chat.ChatEngine
}

// WebhookPayload represents the incoming webhook payload from WhatsApp
type WebhookPayload struct {
	Object string `json:"object"`
	Entry  []struct {
		ID      string `json:"id"`
		Changes []struct {
			Value struct {
				MessagingProduct string `json:"messaging_product"`
				Metadata         struct {
					DisplayPhoneNumber string `json:"display_phone_number"`
					PhoneNumberID      string `json:"phone_number_id"`
				} `json:"metadata"`
				Contacts []struct {
					Profile struct {
						Name string `json:"name"`
					} `json:"profile"`
					WaID string `json:"wa_id"`
				} `json:"contacts"`
				Messages []struct {
					From      string `json:"from"`
					ID        string `json:"id"`
					Timestamp string `json:"timestamp"`
					Type      string `json:"type"`
					Text      *struct {
						Body string `json:"body"`
					} `json:"text,omitempty"`
					Image *struct {
						ID       string `json:"id"`
						MIMEType string `json:"mime_type"`
						SHA256   string `json:"sha256"`
						Caption  string `json:"caption,omitempty"`
					} `json:"image,omitempty"`
					Document *struct {
						ID       string `json:"id"`
						MIMEType string `json:"mime_type"`
						SHA256   string `json:"sha256"`
						Filename string `json:"filename"`
						Caption  string `json:"caption,omitempty"`
					} `json:"document,omitempty"`
					Audio *struct {
						ID       string `json:"id"`
						MIMEType string `json:"mime_type"`
					} `json:"audio,omitempty"`
					Video *struct {
						ID       string `json:"id"`
						MIMEType string `json:"mime_type"`
						Caption  string `json:"caption,omitempty"`
					} `json:"video,omitempty"`
					Voice *struct {
						ID       string `json:"id"`
						MIMEType string `json:"mime_type"`
					} `json:"voice,omitempty"`
					Sticker *struct {
						ID       string `json:"id"`
						MIMEType string `json:"mime_type"`
					} `json:"sticker,omitempty"`
				} `json:"messages"`
			} `json:"value"`
			Field string `json:"field"`
		} `json:"changes"`
	} `json:"entry"`
}

// SendMessageRequest represents the request body for sending a text message
type SendMessageRequest struct {
	MessagingProduct string `json:"messaging_product"`
	RecipientType    string `json:"recipient_type"`
	To               string `json:"to"`
	Type             string `json:"type"`
	Text             struct {
		PreviewURL bool   `json:"preview_url"`
		Body       string `json:"body"`
	} `json:"text"`
}

// NewWhatsAppBot creates a new WhatsApp bot instance
func NewWhatsAppBot(accessToken, verifyToken, appSecret, phoneNumberID string, log *slog.Logger) *WhatsAppBot {
	return &WhatsAppBot{
		log:           log.With(sl.Module("whatsappbot")),
		accessToken:   accessToken,
		verifyToken:   verifyToken,
		appSecret:     appSecret,
		phoneNumberID: phoneNumberID,
	}
}

// SetChatEngine sets the unified chat engine for this bot.
func (b *WhatsAppBot) SetChatEngine(engine *chat.ChatEngine) {
	b.chatEngine = engine
}

// HandleWebhookVerification handles the GET request for webhook verification
func (b *WhatsAppBot) HandleWebhookVerification(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("hub.mode")
	token := r.URL.Query().Get("hub.verify_token")
	challenge := r.URL.Query().Get("hub.challenge")

	if mode == "subscribe" && token == b.verifyToken {
		b.log.Info("webhook verified")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(challenge))
		return
	}

	b.log.Warn("webhook verification failed",
		slog.String("mode", mode),
		slog.Bool("token_match", token == b.verifyToken),
	)
	http.Error(w, "Forbidden", http.StatusForbidden)
}

// HandleWebhook handles incoming webhook POST requests
func (b *WhatsAppBot) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		b.log.Error("failed to read request body", sl.Err(err))
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	b.log.Debug("webhook payload", slog.String("body", string(body)))

	// Verify signature if app secret is configured
	if b.appSecret != "" {
		signature := r.Header.Get("X-Hub-Signature-256")
		if !b.verifySignature(body, signature) {
			b.log.Warn("invalid webhook signature")
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	}

	var payload WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		b.log.Error("failed to parse webhook payload", sl.Err(err))
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// Always respond with 200 OK to acknowledge receipt
	w.WriteHeader(http.StatusOK)

	// Process messages asynchronously
	go b.processPayload(payload)
}

// processPayload processes the webhook payload
func (b *WhatsAppBot) processPayload(payload WebhookPayload) {
	if payload.Object != "whatsapp_business_account" {
		return
	}

	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			if change.Field != "messages" {
				continue
			}

			for _, message := range change.Value.Messages {
				senderPhone := message.From

				// Handle media messages
				var mediaID, mimeType, filename, caption string
				switch message.Type {
				case "image":
					if message.Image != nil {
						mediaID = message.Image.ID
						mimeType = message.Image.MIMEType
						filename = "image.jpg"
						caption = message.Image.Caption
					}
				case "document":
					if message.Document != nil {
						mediaID = message.Document.ID
						mimeType = message.Document.MIMEType
						filename = message.Document.Filename
						caption = message.Document.Caption
					}
				case "audio":
					if message.Audio != nil {
						mediaID = message.Audio.ID
						mimeType = message.Audio.MIMEType
						filename = "audio.ogg"
					}
				case "video":
					if message.Video != nil {
						mediaID = message.Video.ID
						mimeType = message.Video.MIMEType
						filename = "video.mp4"
						caption = message.Video.Caption
					}
				case "voice":
					if message.Voice != nil {
						mediaID = message.Voice.ID
						mimeType = message.Voice.MIMEType
						filename = "voice.ogg"
					}
				case "sticker":
					if message.Sticker != nil {
						mediaID = message.Sticker.ID
						mimeType = message.Sticker.MIMEType
						filename = "sticker.webp"
					}
				}

				if mediaID != "" {
					if b.chatEngine != nil {
						if listener := b.chatEngine.GetMessageListener(); listener != nil {
							b.downloadAndUploadMedia(listener, senderPhone, mediaID, mimeType, filename, caption)
						}
					}
					continue
				}

				// Handle text messages
				if message.Type == "text" && message.Text != nil && message.Text.Body != "" {
					text := message.Text.Body

					// Save incoming message for CRM
					if b.chatEngine != nil {
						if listener := b.chatEngine.GetMessageListener(); listener != nil {
							listener.SaveAndBroadcastChatMessage(entity.ChatMessage{
								Platform:  "whatsapp",
								UserID:    senderPhone,
								ChatID:    senderPhone,
								Direction: "incoming",
								Sender:    "user",
								Text:      text,
								CreatedAt: time.Now(),
							})
						}
					}

					// Delegate to ChatEngine if available
					if b.chatEngine != nil {
						messenger := wamessenger.NewMessenger(b)
						if err := b.chatEngine.HandleMessage(context.Background(), messenger, "whatsapp", senderPhone, senderPhone, text); err != nil {
							b.log.Error("chat engine error",
								slog.String("sender_phone", senderPhone),
								sl.Err(err),
							)
						}
						continue
					}

					// Fallback: echo
					echoText := fmt.Sprintf("Echo: %s", text)
					if err := b.SendMessage(senderPhone, echoText); err != nil {
						b.log.Error("failed to send echo message",
							slog.String("sender_phone", senderPhone),
							sl.Err(err),
						)
					}
				}
			}
		}
	}
}

// SendMessage sends a text message to the specified recipient
func (b *WhatsAppBot) SendMessage(recipientPhone, text string) error {
	reqBody := SendMessageRequest{
		MessagingProduct: "whatsapp",
		RecipientType:    "individual",
		To:               recipientPhone,
		Type:             "text",
	}
	reqBody.Text.PreviewURL = false
	reqBody.Text.Body = text

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/%s/messages", graphAPIURL, b.phoneNumberID)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+b.accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	b.log.Info("message sent successfully", slog.String("recipient_phone", recipientPhone))
	return nil
}

// downloadAndUploadMedia downloads a media file from WhatsApp and uploads it to GridFS.
func (b *WhatsAppBot) downloadAndUploadMedia(listener chat.MessageListener, senderPhone, mediaID, mimeType, filename, caption string) {
	// Step 1: Get media URL from WhatsApp
	mediaURL := fmt.Sprintf("%s/%s", graphAPIURL, mediaID)
	req, err := http.NewRequest(http.MethodGet, mediaURL, nil)
	if err != nil {
		b.log.Error("failed to create media request", slog.String("media_id", mediaID), sl.Err(err))
		return
	}
	req.Header.Set("Authorization", "Bearer "+b.accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		b.log.Error("failed to get media URL", slog.String("media_id", mediaID), sl.Err(err))
		return
	}
	defer resp.Body.Close()

	var mediaInfo struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&mediaInfo); err != nil {
		b.log.Error("failed to decode media info", slog.String("media_id", mediaID), sl.Err(err))
		return
	}

	// Step 2: Download the actual file
	dlReq, err := http.NewRequest(http.MethodGet, mediaInfo.URL, nil)
	if err != nil {
		b.log.Error("failed to create download request", sl.Err(err))
		return
	}
	dlReq.Header.Set("Authorization", "Bearer "+b.accessToken)

	dlResp, err := http.DefaultClient.Do(dlReq)
	if err != nil {
		b.log.Error("failed to download media file", sl.Err(err))
		return
	}
	defer dlResp.Body.Close()

	if mimeType == "" {
		mimeType = dlResp.Header.Get("Content-Type")
	}

	if err := listener.UploadAndSaveFile("whatsapp", senderPhone, dlResp.Body, filename, mimeType, dlResp.ContentLength, caption); err != nil {
		b.log.Error("failed to upload WhatsApp media",
			slog.String("sender_phone", senderPhone),
			sl.Err(err),
		)
	}
}

// SendMediaMessage sends a media message to a recipient via WhatsApp Cloud API.
func (b *WhatsAppBot) SendMediaMessage(recipientPhone, mediaType, mediaURL, caption, filename string) error {
	media := map[string]interface{}{
		"link": mediaURL,
	}
	if caption != "" {
		media["caption"] = caption
	}
	if filename != "" && mediaType == "document" {
		media["filename"] = filename
	}

	payload := map[string]interface{}{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"to":                recipientPhone,
		"type":              mediaType,
		mediaType:           media,
	}

	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal media request: %w", err)
	}

	apiURL := fmt.Sprintf("%s/%s/messages", graphAPIURL, b.phoneNumberID)
	req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+b.accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send media message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// verifySignature verifies the X-Hub-Signature-256 header
func (b *WhatsAppBot) verifySignature(body []byte, signature string) bool {
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

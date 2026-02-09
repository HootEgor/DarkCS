package insta

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"DarkCS/internal/lib/sl"
)

const graphAPIURL = "https://graph.instagram.com/v24.0/me/messages"

// InstaBot handles Instagram messaging via the Graph API
type InstaBot struct {
	log         *slog.Logger
	accessToken string
	verifyToken string
	appSecret   string
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
				Mid    string `json:"mid"`
				Text   string `json:"text"`
				IsEcho bool   `json:"is_echo,omitempty"`
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

// NewInstaBot creates a new Instagram bot instance
func NewInstaBot(accessToken, verifyToken, appSecret string, log *slog.Logger) *InstaBot {
	return &InstaBot{
		log:         log.With(sl.Module("instabot")),
		accessToken: accessToken,
		verifyToken: verifyToken,
		appSecret:   appSecret,
	}
}

// HandleWebhookVerification handles the GET request for webhook verification
func (b *InstaBot) HandleWebhookVerification(w http.ResponseWriter, r *http.Request) {
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
func (b *InstaBot) HandleWebhook(w http.ResponseWriter, r *http.Request) {
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

// processPayload processes the webhook payload and sends echo responses
func (b *InstaBot) processPayload(payload WebhookPayload) {
	if payload.Object != "instagram" {
		return
	}

	for _, entry := range payload.Entry {
		for _, messaging := range entry.Messaging {
			if messaging.Message != nil && messaging.Message.Text != "" && !messaging.Message.IsEcho {
				senderID := messaging.Sender.ID
				text := messaging.Message.Text

				b.log.Info("received message",
					slog.String("sender_id", senderID),
					slog.String("text", text),
				)

				// Echo the message back
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

	url := fmt.Sprintf("%s?access_token=%s", graphAPIURL, b.accessToken)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	b.log.Info("message sent successfully", slog.String("recipient_id", recipientID))
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

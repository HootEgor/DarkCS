package whatsapp

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

const graphAPIURL = "https://graph.facebook.com/v21.0"

// WhatsAppBot handles WhatsApp messaging via the Graph API
type WhatsAppBot struct {
	log           *slog.Logger
	accessToken   string
	verifyToken   string
	appSecret     string
	phoneNumberID string
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

// processPayload processes the webhook payload and sends echo responses
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
				if message.Type == "text" && message.Text != nil && message.Text.Body != "" {
					senderPhone := message.From
					text := message.Text.Body

					b.log.Info("received message",
						slog.String("sender_phone", senderPhone),
						slog.String("text", text),
					)

					// Echo the message back
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

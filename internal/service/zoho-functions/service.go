// Package zoho_functions provides a service for sending buffered chat messages
// to Zoho CRM via the Zoho Functions webhook (API key auth, separate from ZohoService).
package zoho_functions

import (
	"DarkCS/entity"
	"DarkCS/internal/lib/sl"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

// ZohoFunctionsService sends chat messages to Zoho Functions in batches.
// Messages are buffered per contact and flushed every 2 minutes.
type ZohoFunctionsService struct {
	msgURL     string
	apiKey     string
	httpClient *http.Client
	log        *slog.Logger
	msgBuffer  *messageBuffer
}

// NewZohoFunctionsService creates the service and starts the background flush goroutine.
func NewZohoFunctionsService(msgURL, apiKey string, log *slog.Logger) *ZohoFunctionsService {
	s := &ZohoFunctionsService{
		msgURL:     msgURL,
		apiKey:     apiKey,
		httpClient: &http.Client{},
		log:        log.With(sl.Module("zoho-functions")),
		msgBuffer:  newMessageBuffer(),
	}

	s.msgBuffer.Start(func(contactID string, items []entity.ZohoMessageItem) {
		if err := s.SendMessages(contactID, items); err != nil {
			s.log.Error("flush messages failed",
				slog.String("contact_id", contactID),
				slog.Int("count", len(items)),
				slog.String("error", err.Error()),
			)
		}
	})

	return s
}

// BufferMessage adds a message to the per-contact buffer for batched sending.
func (s *ZohoFunctionsService) BufferMessage(contactID string, item entity.ZohoMessageItem) {
	s.msgBuffer.Add(contactID, item)
}

// SendMessages sends a batch of messages for a contact to the Zoho Functions webhook.
func (s *ZohoFunctionsService) SendMessages(contactID string, messages []entity.ZohoMessageItem) error {
	if len(messages) == 0 {
		return nil
	}

	payload := entity.ZohoMessagePayload{
		ContactID: contactID,
		Messages:  messages,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal message payload: %w", err)
	}

	if err := s.doRequest(body); err != nil {
		return fmt.Errorf("send messages to Zoho: %w", err)
	}

	s.log.Debug("messages sent",
		slog.String("contact_id", contactID),
		slog.Int("count", len(messages)),
	)

	return nil
}

// doRequest performs a POST to the Zoho Functions webhook with API key auth.
func (s *ZohoFunctionsService) doRequest(body []byte) error {
	url := s.msgURL + "?auth_type=apikey&zapikey=" + s.apiKey

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("zoho functions returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

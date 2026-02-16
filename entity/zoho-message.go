package entity

// ZohoMessageItem represents a single chat message to be sent to Zoho Functions.
type ZohoMessageItem struct {
	MessageID string `json:"message_id"`
	ChatID    string `json:"chat_id"`
	Content   string `json:"content"`
	Sender    string `json:"sender"`
}

// ZohoMessagePayload is the request body sent to the Zoho Functions webhook.
type ZohoMessagePayload struct {
	ContactID string            `json:"contact_id"`
	Messages  []ZohoMessageItem `json:"messages"`
}

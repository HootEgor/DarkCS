package chat

import (
	"io"

	"DarkCS/entity"
)

// MessageListener is called when a new incoming message is received from any platform.
// This allows saving messages to the database and broadcasting to WebSocket
// without creating circular imports between bot packages and core.
type MessageListener interface {
	SaveAndBroadcastChatMessage(msg entity.ChatMessage)
	// UpdateUserPlatformInfo saves a platform-specific username (e.g. @username) for the user.
	UpdateUserPlatformInfo(platform, userID, messengerName string)
	// UploadAndSaveFile uploads a file to GridFS, saves a ChatMessage with the attachment, and broadcasts via WebSocket.
	UploadAndSaveFile(platform, userID string, reader io.Reader, filename, mimeType string, size int64, caption string) error
}

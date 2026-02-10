package chat

import "DarkCS/entity"

// MessageListener is called when a new incoming message is received from any platform.
// This allows saving messages to the database and broadcasting to WebSocket
// without creating circular imports between bot packages and core.
type MessageListener interface {
	SaveAndBroadcastChatMessage(msg entity.ChatMessage)
}

package chat

import "context"

// ChatStateRepository defines the database operations for chat state.
type ChatStateRepository interface {
	SaveChatState(ctx context.Context, state *ChatState) error
	LoadChatState(ctx context.Context, platform, userID string) (*ChatState, error)
	DeleteChatState(ctx context.Context, platform, userID string) error
}

// MongoChatStateStorage adapts the database repository to the ChatStateStorage interface.
type MongoChatStateStorage struct {
	repo ChatStateRepository
}

// NewMongoChatStateStorage creates a new MongoDB chat state storage.
func NewMongoChatStateStorage(repo ChatStateRepository) *MongoChatStateStorage {
	return &MongoChatStateStorage{repo: repo}
}

func (s *MongoChatStateStorage) Save(ctx context.Context, state *ChatState) error {
	return s.repo.SaveChatState(ctx, state)
}

func (s *MongoChatStateStorage) Load(ctx context.Context, platform, userID string) (*ChatState, error) {
	return s.repo.LoadChatState(ctx, platform, userID)
}

func (s *MongoChatStateStorage) Delete(ctx context.Context, platform, userID string) error {
	return s.repo.DeleteChatState(ctx, platform, userID)
}

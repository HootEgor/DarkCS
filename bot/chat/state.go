package chat

import "time"

// ChatState represents the platform-agnostic workflow state for a user.
type ChatState struct {
	Platform    string         `json:"platform" bson:"platform"`
	UserID      string         `json:"user_id" bson:"user_id"`
	ChatID      string         `json:"chat_id" bson:"chat_id"`
	WorkflowID  WorkflowID     `json:"workflow_id" bson:"workflow_id"`
	CurrentStep StepID         `json:"current_step" bson:"current_step"`
	Data        map[string]any `json:"data" bson:"data"`
	UpdatedAt   time.Time      `json:"updated_at" bson:"updated_at"`
}

// NewChatState creates a new ChatState with default values.
func NewChatState(platform, userID, chatID string, workflowID WorkflowID, initialStep StepID) *ChatState {
	return &ChatState{
		Platform:    platform,
		UserID:      userID,
		ChatID:      chatID,
		WorkflowID:  workflowID,
		CurrentStep: initialStep,
		Data:        make(map[string]any),
		UpdatedAt:   time.Now(),
	}
}

// GetString retrieves a string value from the state data.
func (s *ChatState) GetString(key string) string {
	if v, ok := s.Data[key]; ok {
		if str, ok := v.(string); ok {
			return str
		}
	}
	return ""
}

// GetInt retrieves an integer value from the state data.
func (s *ChatState) GetInt(key string) int {
	if v, ok := s.Data[key]; ok {
		switch val := v.(type) {
		case int:
			return val
		case int32:
			return int(val)
		case int64:
			return int(val)
		case float64:
			return int(val)
		}
	}
	return 0
}

// GetBool retrieves a boolean value from the state data.
func (s *ChatState) GetBool(key string) bool {
	if v, ok := s.Data[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// Set stores a value in the state data.
func (s *ChatState) Set(key string, value any) {
	if s.Data == nil {
		s.Data = make(map[string]any)
	}
	s.Data[key] = value
}

// MergeData merges additional data into the state.
func (s *ChatState) MergeData(data map[string]any) {
	if s.Data == nil {
		s.Data = make(map[string]any)
	}
	for k, v := range data {
		s.Data[k] = v
	}
}

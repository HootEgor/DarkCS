package workflow

import "time"

// DeepLinkData represents parsed deep link information.
type DeepLinkData struct {
	Type  string `json:"type" bson:"type"`   // e.g., "school"
	Code  string `json:"code" bson:"code"`   // e.g., "abc123"
	Extra string `json:"extra" bson:"extra"` // any additional data
}

// PaginationState tracks the current pagination position.
type PaginationState struct {
	CurrentPage  int `json:"current_page" bson:"current_page"`
	TotalPages   int `json:"total_pages" bson:"total_pages"`
	TotalItems   int `json:"total_items" bson:"total_items"`
	ItemsPerPage int `json:"items_per_page" bson:"items_per_page"`
}

// UserState represents the current workflow state for a user.
type UserState struct {
	UserID      int64            `json:"user_id" bson:"user_id"`
	ChatID      int64            `json:"chat_id" bson:"chat_id"`
	WorkflowID  WorkflowID       `json:"workflow_id" bson:"workflow_id"`
	CurrentStep StepID           `json:"current_step" bson:"current_step"`
	DeepCode    string           `json:"deep_code" bson:"deep_code"`
	DeepLink    *DeepLinkData    `json:"deep_link" bson:"deep_link"`
	Data        map[string]any   `json:"data" bson:"data"`
	Pagination  *PaginationState `json:"pagination" bson:"pagination"`
	UpdatedAt   time.Time        `json:"updated_at" bson:"updated_at"`
}

// NewUserState creates a new UserState with default values.
func NewUserState(userID, chatID int64, workflowID WorkflowID, initialStep StepID) *UserState {
	return &UserState{
		UserID:      userID,
		ChatID:      chatID,
		WorkflowID:  workflowID,
		CurrentStep: initialStep,
		Data:        make(map[string]any),
		UpdatedAt:   time.Now(),
	}
}

// SetDeepLink sets the deep link data for the state.
func (s *UserState) SetDeepLink(dl *DeepLinkData) {
	s.DeepLink = dl
	if dl != nil {
		s.DeepCode = dl.Type + "_" + dl.Code
	}
}

// GetString retrieves a string value from the state data.
func (s *UserState) GetString(key string) string {
	if v, ok := s.Data[key]; ok {
		if str, ok := v.(string); ok {
			return str
		}
	}
	return ""
}

// GetInt retrieves an integer value from the state data.
func (s *UserState) GetInt(key string) int {
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
func (s *UserState) GetBool(key string) bool {
	if v, ok := s.Data[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// Set stores a value in the state data.
func (s *UserState) Set(key string, value any) {
	if s.Data == nil {
		s.Data = make(map[string]any)
	}
	s.Data[key] = value
}

// MergeData merges additional data into the state.
func (s *UserState) MergeData(data map[string]any) {
	if s.Data == nil {
		s.Data = make(map[string]any)
	}
	for k, v := range data {
		s.Data[k] = v
	}
}

// InitPagination initializes pagination state.
func (s *UserState) InitPagination(totalItems, itemsPerPage int) {
	totalPages := totalItems / itemsPerPage
	if totalItems%itemsPerPage > 0 {
		totalPages++
	}
	if totalPages == 0 {
		totalPages = 1
	}
	s.Pagination = &PaginationState{
		CurrentPage:  1,
		TotalPages:   totalPages,
		TotalItems:   totalItems,
		ItemsPerPage: itemsPerPage,
	}
}

// NextPage moves to the next page if available.
func (s *UserState) NextPage() bool {
	if s.Pagination == nil {
		return false
	}
	if s.Pagination.CurrentPage < s.Pagination.TotalPages {
		s.Pagination.CurrentPage++
		return true
	}
	return false
}

// PrevPage moves to the previous page if available.
func (s *UserState) PrevPage() bool {
	if s.Pagination == nil {
		return false
	}
	if s.Pagination.CurrentPage > 1 {
		s.Pagination.CurrentPage--
		return true
	}
	return false
}

// GetPageItems returns the start and end indices for current page items.
func (s *UserState) GetPageItems() (start, end int) {
	if s.Pagination == nil {
		return 0, 0
	}
	start = (s.Pagination.CurrentPage - 1) * s.Pagination.ItemsPerPage
	end = start + s.Pagination.ItemsPerPage
	if end > s.Pagination.TotalItems {
		end = s.Pagination.TotalItems
	}
	return start, end
}

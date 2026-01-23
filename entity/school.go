package entity

import "time"

// School represents a school entity for the onboarding workflow.
type School struct {
	ID        string    `json:"id" bson:"_id"`
	Name      string    `json:"name" bson:"name"`
	Active    bool      `json:"active" bson:"active"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
}

// NewSchool creates a new School entity.
func NewSchool(id, name, code string) *School {
	return &School{
		ID:        id,
		Name:      name,
		Active:    true,
		CreatedAt: time.Now(),
	}
}

// IsActive checks if the school is active.
func (s *School) IsActive() bool {
	return s.Active
}

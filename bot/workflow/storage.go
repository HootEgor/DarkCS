package workflow

import (
	"context"
)

// MongoStateStorage is an adapter that wraps the database operations.
type MongoStateStorage struct {
	repo StateRepository
}

// StateRepository defines the database operations for workflow state.
type StateRepository interface {
	SaveWorkflowState(ctx context.Context, state *UserState) error
	LoadWorkflowState(ctx context.Context, userID int64) (*UserState, error)
	DeleteWorkflowState(ctx context.Context, userID int64) error
	WorkflowStateExists(ctx context.Context, userID int64) (bool, error)
}

// NewMongoStateStorage creates a new MongoDB state storage.
func NewMongoStateStorage(repo StateRepository) *MongoStateStorage {
	return &MongoStateStorage{repo: repo}
}

// Save persists a user's workflow state.
func (s *MongoStateStorage) Save(ctx context.Context, state *UserState) error {
	return s.repo.SaveWorkflowState(ctx, state)
}

// Load retrieves a user's workflow state.
func (s *MongoStateStorage) Load(ctx context.Context, userID int64) (*UserState, error) {
	return s.repo.LoadWorkflowState(ctx, userID)
}

// Delete removes a user's workflow state.
func (s *MongoStateStorage) Delete(ctx context.Context, userID int64) error {
	return s.repo.DeleteWorkflowState(ctx, userID)
}

// Exists checks if a user has a saved state.
func (s *MongoStateStorage) Exists(ctx context.Context, userID int64) (bool, error) {
	return s.repo.WorkflowStateExists(ctx, userID)
}

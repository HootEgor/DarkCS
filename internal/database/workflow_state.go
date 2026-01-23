package repository

import (
	"DarkCS/bot/workflow"
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const workflowStatesCollection = "workflow_states"

// SaveWorkflowState persists a user's workflow state.
func (m *MongoDB) SaveWorkflowState(ctx context.Context, state *workflow.UserState) error {
	connection, err := m.connect()
	if err != nil {
		return err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(workflowStatesCollection)

	state.UpdatedAt = time.Now()

	filter := bson.D{{"user_id", state.UserID}}
	update := bson.D{{"$set", state}}
	opts := options.Update().SetUpsert(true)

	_, err = collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return err
	}

	return nil
}

// LoadWorkflowState retrieves a user's workflow state.
func (m *MongoDB) LoadWorkflowState(ctx context.Context, userID int64) (*workflow.UserState, error) {
	connection, err := m.connect()
	if err != nil {
		return nil, err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(workflowStatesCollection)

	filter := bson.D{{"user_id", userID}}

	var state workflow.UserState
	err = collection.FindOne(ctx, filter).Decode(&state)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}

	return &state, nil
}

// DeleteWorkflowState removes a user's workflow state.
func (m *MongoDB) DeleteWorkflowState(ctx context.Context, userID int64) error {
	connection, err := m.connect()
	if err != nil {
		return err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(workflowStatesCollection)

	filter := bson.D{{"user_id", userID}}

	_, err = collection.DeleteOne(ctx, filter)
	return err
}

// WorkflowStateExists checks if a user has a saved workflow state.
func (m *MongoDB) WorkflowStateExists(ctx context.Context, userID int64) (bool, error) {
	connection, err := m.connect()
	if err != nil {
		return false, err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(workflowStatesCollection)

	filter := bson.D{{"user_id", userID}}

	count, err := collection.CountDocuments(ctx, filter)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

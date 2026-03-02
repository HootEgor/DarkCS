package repository

import (
	"DarkCS/bot/chat"
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const chatStatesCollection = "chat_states"

// SaveChatState persists a user's chat state by {platform, user_id}.
func (m *MongoDB) SaveChatState(ctx context.Context, state *chat.ChatState) error {
	connection, err := m.connect()
	if err != nil {
		return err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(chatStatesCollection)

	state.UpdatedAt = time.Now()

	filter := bson.D{{"platform", state.Platform}, {"user_id", state.UserID}}
	update := bson.D{{"$set", state}}
	opts := options.Update().SetUpsert(true)

	_, err = collection.UpdateOne(ctx, filter, update, opts)
	return err
}

// LoadChatState retrieves a user's chat state by {platform, user_id}.
func (m *MongoDB) LoadChatState(ctx context.Context, platform, userID string) (*chat.ChatState, error) {
	connection, err := m.connect()
	if err != nil {
		return nil, err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(chatStatesCollection)

	filter := bson.D{{"platform", platform}, {"user_id", userID}}

	var state chat.ChatState
	err = collection.FindOne(ctx, filter).Decode(&state)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}

	return &state, nil
}

// FindChatStatesBySteps returns all chat states in the given workflow whose current step
// matches any of the provided step IDs. Used for admin bulk operations.
func (m *MongoDB) FindChatStatesBySteps(ctx context.Context, workflowID chat.WorkflowID, steps []chat.StepID) ([]*chat.ChatState, error) {
	connection, err := m.connect()
	if err != nil {
		return nil, err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(chatStatesCollection)

	stepValues := make(bson.A, len(steps))
	for i, s := range steps {
		stepValues[i] = string(s)
	}

	filter := bson.D{
		{"workflow_id", string(workflowID)},
		{"current_step", bson.D{{"$in", stepValues}}},
	}

	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var states []*chat.ChatState
	if err := cursor.All(ctx, &states); err != nil {
		return nil, err
	}

	return states, nil
}

// DeleteChatState removes a user's chat state by {platform, user_id}.
func (m *MongoDB) DeleteChatState(ctx context.Context, platform, userID string) error {
	connection, err := m.connect()
	if err != nil {
		return err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(chatStatesCollection)

	filter := bson.D{{"platform", platform}, {"user_id", userID}}

	_, err = collection.DeleteOne(ctx, filter)
	return err
}

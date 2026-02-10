package repository

import (
	"DarkCS/entity"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SaveChatMessage inserts a chat message and trims to 100 per user.
func (m *MongoDB) SaveChatMessage(msg entity.ChatMessage) error {
	connection, err := m.connect()
	if err != nil {
		return err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(chatMessagesCollection)

	_, err = collection.InsertOne(m.ctx, msg)
	if err != nil {
		return fmt.Errorf("mongodb insert chat message: %w", err)
	}

	// Trim to 100 messages per user
	filter := bson.D{{"platform", msg.Platform}, {"user_id", msg.UserID}}
	count, err := collection.CountDocuments(m.ctx, filter)
	if err != nil {
		return fmt.Errorf("mongodb count chat messages: %w", err)
	}

	if count > 100 {
		// Find the 100th newest message's created_at
		opts := options.FindOne().SetSort(bson.D{{"created_at", -1}}).SetSkip(99)
		var cutoff entity.ChatMessage
		err = collection.FindOne(m.ctx, filter, opts).Decode(&cutoff)
		if err != nil {
			return fmt.Errorf("mongodb find cutoff message: %w", err)
		}

		// Delete all older messages for this user
		deleteFilter := bson.D{
			{"platform", msg.Platform},
			{"user_id", msg.UserID},
			{"created_at", bson.D{{"$lt", cutoff.CreatedAt}}},
		}
		_, err = collection.DeleteMany(m.ctx, deleteFilter)
		if err != nil {
			return fmt.Errorf("mongodb trim chat messages: %w", err)
		}
	}

	return nil
}

// GetChatMessages returns messages for a user, paginated (newest first).
func (m *MongoDB) GetChatMessages(platform, userID string, limit, offset int) ([]entity.ChatMessage, error) {
	connection, err := m.connect()
	if err != nil {
		return nil, err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(chatMessagesCollection)

	filter := bson.D{{"platform", platform}, {"user_id", userID}}
	opts := options.Find().
		SetSort(bson.D{{"created_at", -1}}).
		SetLimit(int64(limit)).
		SetSkip(int64(offset))

	cursor, err := collection.Find(m.ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("mongodb find chat messages: %w", err)
	}
	defer cursor.Close(m.ctx)

	var messages []entity.ChatMessage
	if err = cursor.All(m.ctx, &messages); err != nil {
		return nil, fmt.Errorf("mongodb decode chat messages: %w", err)
	}

	return messages, nil
}

// GetActiveChats returns chat summaries with last message info.
func (m *MongoDB) GetActiveChats() ([]entity.ChatSummary, error) {
	connection, err := m.connect()
	if err != nil {
		return nil, err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(chatMessagesCollection)

	pipeline := mongo.Pipeline{
		// Sort by created_at descending so $first gives the latest message
		{{Key: "$sort", Value: bson.D{{"created_at", -1}}}},
		// Group by (platform, user_id) to get last message per user
		{{Key: "$group", Value: bson.D{
			{"_id", bson.D{{"platform", "$platform"}, {"user_id", "$user_id"}}},
			{"last_message", bson.D{{"$first", "$text"}}},
			{"last_time", bson.D{{"$first", "$created_at"}}},
			{"unread", bson.D{{"$sum", bson.D{
				{"$cond", bson.A{
					bson.D{{"$eq", bson.A{"$direction", "incoming"}}},
					1,
					0,
				}},
			}}}},
		}}},
		// Sort by last_time descending
		{{Key: "$sort", Value: bson.D{{"last_time", -1}}}},
		// Reshape output
		{{Key: "$project", Value: bson.D{
			{"_id", 0},
			{"platform", "$_id.platform"},
			{"user_id", "$_id.user_id"},
			{"last_message", 1},
			{"last_time", 1},
			{"unread", 1},
		}}},
	}

	cursor, err := collection.Aggregate(m.ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("mongodb aggregate active chats: %w", err)
	}
	defer cursor.Close(m.ctx)

	var summaries []entity.ChatSummary
	if err = cursor.All(m.ctx, &summaries); err != nil {
		return nil, fmt.Errorf("mongodb decode chat summaries: %w", err)
	}

	return summaries, nil
}

// CleanupChatMessages deletes messages older than 30 days, keeping at least 20 per user.
func (m *MongoDB) CleanupChatMessages() error {
	connection, err := m.connect()
	if err != nil {
		return err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(chatMessagesCollection)

	cutoffDate := time.Now().AddDate(0, 0, -30)

	// Get all users with their message counts
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{"_id", bson.D{{"platform", "$platform"}, {"user_id", "$user_id"}}},
			{"count", bson.D{{"$sum", 1}}},
		}}},
		{{Key: "$match", Value: bson.D{
			{"count", bson.D{{"$gt", 20}}},
		}}},
	}

	cursor, err := collection.Aggregate(m.ctx, pipeline)
	if err != nil {
		return fmt.Errorf("mongodb aggregate for cleanup: %w", err)
	}
	defer cursor.Close(m.ctx)

	type userGroup struct {
		ID struct {
			Platform string `bson:"platform"`
			UserID   string `bson:"user_id"`
		} `bson:"_id"`
		Count int `bson:"count"`
	}

	for cursor.Next(m.ctx) {
		var group userGroup
		if err := cursor.Decode(&group); err != nil {
			continue
		}

		maxDeletable := group.Count - 20

		// Find old messages to delete
		findFilter := bson.D{
			{"platform", group.ID.Platform},
			{"user_id", group.ID.UserID},
			{"created_at", bson.D{{"$lt", cutoffDate}}},
		}
		findOpts := options.Find().
			SetSort(bson.D{{"created_at", 1}}).
			SetLimit(int64(maxDeletable)).
			SetProjection(bson.D{{"_id", 1}})

		oldCursor, err := collection.Find(m.ctx, findFilter, findOpts)
		if err != nil {
			continue
		}

		var ids []interface{}
		for oldCursor.Next(m.ctx) {
			var doc struct {
				ID interface{} `bson:"_id"`
			}
			if err := oldCursor.Decode(&doc); err == nil {
				ids = append(ids, doc.ID)
			}
		}
		oldCursor.Close(m.ctx)

		if len(ids) > 0 {
			deleteFilter := bson.D{{"_id", bson.D{{"$in", ids}}}}
			_, _ = collection.DeleteMany(m.ctx, deleteFilter)
		}
	}

	return nil
}

// EnsureChatMessageIndexes creates indexes for the chat-messages collection.
func (m *MongoDB) EnsureChatMessageIndexes() error {
	connection, err := m.connect()
	if err != nil {
		return err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(chatMessagesCollection)

	index := mongo.IndexModel{
		Keys: bson.D{
			{"platform", 1},
			{"user_id", 1},
			{"created_at", -1},
		},
	}

	_, err = collection.Indexes().CreateOne(m.ctx, index)
	if err != nil {
		return fmt.Errorf("mongodb create chat message index: %w", err)
	}

	return nil
}

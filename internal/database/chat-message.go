package repository

import (
	"DarkCS/entity"
	"errors"
	"fmt"
	"strings"
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

// GetActiveChats returns chat summaries with last message info (without unread counts).
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
		}}},
	}

	// allowDiskUse: the $group can exceed the 100 MB in-memory stage limit on large histories.
	cursor, err := collection.Aggregate(m.ctx, pipeline, options.Aggregate().SetAllowDiskUse(true))
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

// CountUnreadPerChat counts incoming messages after readAt for each chat (keys are
// "platform:user_id"; chats without a receipt count every incoming message).
//
// Counting happens in MongoDB with two aggregations that only return counts: totals for
// every chat, then counts after readAt for the chats that have a receipt. The previous
// version pushed every incoming message timestamp into memory.
func (m *MongoDB) CountUnreadPerChat(receipts map[string]time.Time) (map[string]int, error) {
	collection := m.collection(chatMessagesCollection)

	type chatCount struct {
		ID struct {
			Platform string `bson:"platform"`
			UserID   string `bson:"user_id"`
		} `bson:"_id"`
		Count int `bson:"count"`
	}
	countBy := func(match bson.D) ([]chatCount, error) {
		pipeline := mongo.Pipeline{
			{{Key: "$match", Value: match}},
			{{Key: "$group", Value: bson.D{
				{"_id", bson.D{{"platform", "$platform"}, {"user_id", "$user_id"}}},
				{"count", bson.D{{"$sum", 1}}},
			}}},
		}
		cursor, err := collection.Aggregate(m.ctx, pipeline)
		if err != nil {
			return nil, err
		}
		var counts []chatCount
		if err = cursor.All(m.ctx, &counts); err != nil {
			return nil, err
		}
		return counts, nil
	}

	totals, err := countBy(bson.D{{"direction", "incoming"}})
	if err != nil {
		return nil, fmt.Errorf("mongodb aggregate unread totals: %w", err)
	}

	result := make(map[string]int, len(totals))
	for _, c := range totals {
		key := c.ID.Platform + ":" + c.ID.UserID
		if _, hasReceipt := receipts[key]; !hasReceipt {
			result[key] = c.Count
		}
	}
	if len(receipts) == 0 {
		return result, nil
	}

	// Chats with a receipt: unread starts at 0 and only messages after readAt count.
	var or bson.A
	for key, readAt := range receipts {
		platform, userID, ok := strings.Cut(key, ":")
		if !ok {
			continue
		}
		result[key] = 0
		or = append(or, bson.D{
			{"platform", platform},
			{"user_id", userID},
			{"created_at", bson.D{{"$gt", readAt}}},
		})
	}
	if len(or) == 0 {
		return result, nil
	}
	afterRead, err := countBy(bson.D{{"direction", "incoming"}, {"$or", or}})
	if err != nil {
		return nil, fmt.Errorf("mongodb aggregate unread after read: %w", err)
	}
	for _, c := range afterRead {
		result[c.ID.Platform+":"+c.ID.UserID] = c.Count
	}

	return result, nil
}

// UpsertReadReceipt upserts a read receipt for a CRM user/chat combination.
func (m *MongoDB) UpsertReadReceipt(username, platform, userID string, readAt time.Time) error {
	connection, err := m.connect()
	if err != nil {
		return err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(readReceiptsCollection)

	filter := bson.D{
		{"username", username},
		{"platform", platform},
		{"user_id", userID},
	}
	update := bson.D{{"$set", bson.D{{"read_at", readAt}}}}
	opts := options.Update().SetUpsert(true)

	_, err = collection.UpdateOne(m.ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("mongodb upsert read receipt: %w", err)
	}

	return nil
}

// GetReadReceipts returns all read receipts for a CRM username.
func (m *MongoDB) GetReadReceipts(username string) ([]entity.ChatReadReceipt, error) {
	connection, err := m.connect()
	if err != nil {
		return nil, err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(readReceiptsCollection)

	filter := bson.D{{"username", username}}
	cursor, err := collection.Find(m.ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("mongodb find read receipts: %w", err)
	}
	defer cursor.Close(m.ctx)

	var receipts []entity.ChatReadReceipt
	if err = cursor.All(m.ctx, &receipts); err != nil {
		return nil, fmt.Errorf("mongodb decode read receipts: %w", err)
	}

	return receipts, nil
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

	// Keep going past per-chat failures so one bad chat doesn't block cleanup of the
	// rest, but report everything that failed.
	var errs []error
	for cursor.Next(m.ctx) {
		var group userGroup
		if err := cursor.Decode(&group); err != nil {
			errs = append(errs, fmt.Errorf("decode cleanup group: %w", err))
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
			errs = append(errs, fmt.Errorf("find old messages %s:%s: %w", group.ID.Platform, group.ID.UserID, err))
			continue
		}

		var docs []struct {
			ID interface{} `bson:"_id"`
		}
		if err = oldCursor.All(m.ctx, &docs); err != nil {
			errs = append(errs, fmt.Errorf("read old messages %s:%s: %w", group.ID.Platform, group.ID.UserID, err))
			continue
		}
		if len(docs) == 0 {
			continue
		}

		ids := make([]interface{}, 0, len(docs))
		for _, d := range docs {
			ids = append(ids, d.ID)
		}
		deleteFilter := bson.D{{"_id", bson.D{{"$in", ids}}}}
		if _, err = collection.DeleteMany(m.ctx, deleteFilter); err != nil {
			errs = append(errs, fmt.Errorf("delete old messages %s:%s: %w", group.ID.Platform, group.ID.UserID, err))
		}
	}
	if err = cursor.Err(); err != nil {
		errs = append(errs, fmt.Errorf("cleanup cursor: %w", err))
	}

	return errors.Join(errs...)
}

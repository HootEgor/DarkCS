package repository

import (
	"DarkCS/entity"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// channelFilter matches one chat channel. The default channel is stored as a missing
// field (omitempty), which also covers every document written before channels existed.
func channelFilter(channel string) bson.E {
	if channel == "" {
		return bson.E{Key: "channel", Value: bson.D{{"$exists", false}}}
	}
	return bson.E{Key: "channel", Value: channel}
}

// keepsFullHistory reports whether a platform is exempt from message trimming:
// Telegram Business chats are low-volume B2B conversations and may hold imported history.
func keepsFullHistory(platform string) bool {
	return platform == entity.PlatformTelegramBusiness
}

// SaveChatMessage inserts a chat message and trims the chat to 100 messages
// (except for platforms that keep their full history).
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

	if keepsFullHistory(msg.Platform) {
		return nil
	}

	// Trim to 100 messages per chat
	filter := bson.D{{"platform", msg.Platform}, {"user_id", msg.UserID}, channelFilter(msg.Channel)}
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
			channelFilter(msg.Channel),
			{"created_at", bson.D{{"$lt", cutoff.CreatedAt}}},
		}
		_, err = collection.DeleteMany(m.ctx, deleteFilter)
		if err != nil {
			return fmt.Errorf("mongodb trim chat messages: %w", err)
		}
	}

	return nil
}

// GetChatMessages returns messages of one chat, paginated (newest first).
func (m *MongoDB) GetChatMessages(platform, userID, channel string, limit, offset int) ([]entity.ChatMessage, error) {
	connection, err := m.connect()
	if err != nil {
		return nil, err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(chatMessagesCollection)

	filter := bson.D{{"platform", platform}, {"user_id", userID}, channelFilter(channel)}
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
		// Group by (platform, user_id, channel) to get last message per chat
		{{Key: "$group", Value: bson.D{
			{"_id", bson.D{{"platform", "$platform"}, {"user_id", "$user_id"}, {"channel", "$channel"}}},
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
			{"channel", "$_id.channel"},
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
// entity.ChatKey values; chats without a receipt count every incoming message).
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
			Channel  string `bson:"channel"`
		} `bson:"_id"`
		Count int `bson:"count"`
	}
	countBy := func(match bson.D) ([]chatCount, error) {
		pipeline := mongo.Pipeline{
			{{Key: "$match", Value: match}},
			{{Key: "$group", Value: bson.D{
				{"_id", bson.D{{"platform", "$platform"}, {"user_id", "$user_id"}, {"channel", "$channel"}}},
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
		key := entity.ChatKey(c.ID.Platform, c.ID.UserID, c.ID.Channel)
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
		parts := strings.SplitN(key, ":", 3)
		if len(parts) < 2 {
			continue
		}
		channel := ""
		if len(parts) == 3 {
			channel = parts[2]
		}
		result[key] = 0
		or = append(or, bson.D{
			{"platform", parts[0]},
			{"user_id", parts[1]},
			channelFilter(channel),
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
		result[entity.ChatKey(c.ID.Platform, c.ID.UserID, c.ID.Channel)] = c.Count
	}

	return result, nil
}

// UpsertReadReceipt upserts a read receipt for a CRM user/chat combination.
func (m *MongoDB) UpsertReadReceipt(username, platform, userID, channel string, readAt time.Time) error {
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
		channelFilter(channel),
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

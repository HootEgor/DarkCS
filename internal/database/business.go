package repository

import (
	"DarkCS/entity"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Telegram Business collections: connected premium accounts, cached client profiles and
// the record of imported chat histories.
const (
	businessConnectionsCollection = "tg-business-connections"
	businessContactsCollection    = "tg-business-contacts"
	historyImportsCollection      = "tg-business-history-imports"
)

// UpsertBusinessConnection stores the latest state of a business connection, keyed by
// the owner so a reconnect (new connection id) updates the same document.
func (m *MongoDB) UpsertBusinessConnection(conn entity.BusinessConnection) error {
	_, err := m.collection(businessConnectionsCollection).UpdateOne(m.ctx,
		bson.D{{"owner_user_id", conn.OwnerUserID}},
		bson.D{{"$set", conn}},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("mongodb upsert business connection: %w", err)
	}
	return nil
}

// GetBusinessConnectionByID returns the connection with the given Telegram connection id,
// or nil when it is unknown.
func (m *MongoDB) GetBusinessConnectionByID(connectionID string) (*entity.BusinessConnection, error) {
	return m.findBusinessConnection(bson.D{{"connection_id", connectionID}})
}

// GetBusinessConnectionByOwner returns the connection of a business account owner, or nil.
func (m *MongoDB) GetBusinessConnectionByOwner(ownerUserID int64) (*entity.BusinessConnection, error) {
	return m.findBusinessConnection(bson.D{{"owner_user_id", ownerUserID}})
}

func (m *MongoDB) findBusinessConnection(filter bson.D) (*entity.BusinessConnection, error) {
	var conn entity.BusinessConnection
	err := m.collection(businessConnectionsCollection).FindOne(m.ctx, filter).Decode(&conn)
	if err != nil {
		return nil, m.findError(err)
	}
	return &conn, nil
}

// ListBusinessConnections returns every known business account, enabled or not.
func (m *MongoDB) ListBusinessConnections() ([]entity.BusinessConnection, error) {
	cursor, err := m.collection(businessConnectionsCollection).Find(m.ctx, bson.D{},
		options.Find().SetSort(bson.D{{"owner_name", 1}}))
	if err != nil {
		return nil, fmt.Errorf("mongodb find business connections: %w", err)
	}
	var conns []entity.BusinessConnection
	if err = cursor.All(m.ctx, &conns); err != nil {
		return nil, fmt.Errorf("mongodb decode business connections: %w", err)
	}
	return conns, nil
}

// UpsertBusinessContact caches a client's Telegram profile.
func (m *MongoDB) UpsertBusinessContact(contact entity.BusinessContact) error {
	_, err := m.collection(businessContactsCollection).UpdateOne(m.ctx,
		bson.D{{"user_id", contact.UserID}},
		bson.D{{"$set", contact}},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("mongodb upsert business contact: %w", err)
	}
	return nil
}

// GetBusinessContact returns the cached Telegram profile of a client, or nil.
func (m *MongoDB) GetBusinessContact(userID string) (*entity.BusinessContact, error) {
	var contact entity.BusinessContact
	err := m.collection(businessContactsCollection).FindOne(m.ctx, bson.D{{"user_id", userID}}).Decode(&contact)
	if err != nil {
		return nil, m.findError(err)
	}
	return &contact, nil
}

// GetEarliestLiveMessageTime returns the time of the oldest non-imported message of a
// chat, or nil for an empty chat. Imported history must end before it.
func (m *MongoDB) GetEarliestLiveMessageTime(platform, userID, channel string) (*time.Time, error) {
	filter := bson.D{
		{"platform", platform},
		{"user_id", userID},
		channelFilter(channel),
		{"imported", bson.D{{"$ne", true}}},
	}
	var msg entity.ChatMessage
	err := m.collection(chatMessagesCollection).FindOne(m.ctx, filter,
		options.FindOne().SetSort(bson.D{{"created_at", 1}}).SetProjection(bson.D{{"created_at", 1}}),
	).Decode(&msg)
	if err != nil {
		return nil, m.findError(err)
	}
	return &msg.CreatedAt, nil
}

// InsertImportedMessages bulk-inserts imported history and returns how many were new.
// Messages already stored (same Telegram message id) hit the unique index and are
// skipped; any other write error is returned.
func (m *MongoDB) InsertImportedMessages(msgs []entity.ChatMessage) (int, error) {
	if len(msgs) == 0 {
		return 0, nil
	}
	docs := make([]any, len(msgs))
	for i := range msgs {
		docs[i] = msgs[i]
	}

	res, err := m.collection(chatMessagesCollection).InsertMany(m.ctx, docs, options.InsertMany().SetOrdered(false))
	inserted := 0
	if res != nil {
		inserted = len(res.InsertedIDs)
	}
	if err == nil {
		return inserted, nil
	}

	var bulkErr mongo.BulkWriteException
	if !errors.As(err, &bulkErr) || bulkErr.WriteConcernError != nil {
		return inserted, fmt.Errorf("mongodb insert imported messages: %w", err)
	}
	for _, we := range bulkErr.WriteErrors {
		if we.Code != 11000 { // 11000 = DuplicateKey
			return inserted, fmt.Errorf("mongodb insert imported messages: %w", err)
		}
	}
	// Only duplicates failed. InsertedIDs may list attempted ids, so count from the errors.
	return len(msgs) - len(bulkErr.WriteErrors), nil
}

// businessMessageFilter matches messages of one Telegram Business chat by Telegram message id.
func businessMessageFilter(platform, userID, channel string, tgMessageIDs ...int64) bson.D {
	ids := bson.A{}
	for _, id := range tgMessageIDs {
		ids = append(ids, id)
	}
	return bson.D{
		{"platform", platform},
		{"user_id", userID},
		channelFilter(channel),
		{"tg_message_id", bson.D{{"$in", ids}}},
	}
}

// EditChatMessageText mirrors an edit made in Telegram: it replaces the text, keeps the
// text from before the first edit in original_text and stamps edited_at. Returns the
// updated message, or nil when the message is not stored (e.g. older than the connection).
func (m *MongoDB) EditChatMessageText(platform, userID, channel string, tgMessageID int64, text string, editedAt time.Time) (*entity.ChatMessage, error) {
	// A pipeline update so original_text is only set once; $literal keeps a text that
	// starts with "$" from being read as a field path.
	update := mongo.Pipeline{{{Key: "$set", Value: bson.D{
		{"original_text", bson.D{{"$ifNull", bson.A{"$original_text", "$text"}}}},
		{"text", bson.D{{"$literal", text}}},
		{"edited_at", editedAt},
	}}}}
	var msg entity.ChatMessage
	err := m.collection(chatMessagesCollection).FindOneAndUpdate(m.ctx,
		businessMessageFilter(platform, userID, channel, tgMessageID),
		update,
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&msg)
	if err != nil {
		return nil, m.findError(err)
	}
	return &msg, nil
}

// MarkChatMessagesDeleted soft-deletes messages deleted in Telegram and returns the ids
// of the stored messages that were marked (unknown Telegram ids are ignored).
func (m *MongoDB) MarkChatMessagesDeleted(platform, userID, channel string, tgMessageIDs []int64, deletedAt time.Time) ([]entity.ChatMessage, error) {
	if len(tgMessageIDs) == 0 {
		return nil, nil
	}
	collection := m.collection(chatMessagesCollection)
	filter := businessMessageFilter(platform, userID, channel, tgMessageIDs...)
	filter = append(filter, bson.E{Key: "deleted_at", Value: bson.D{{"$exists", false}}})

	cursor, err := collection.Find(m.ctx, filter, options.Find().SetProjection(bson.D{{"_id", 1}, {"tg_message_id", 1}}))
	if err != nil {
		return nil, fmt.Errorf("mongodb find deleted messages: %w", err)
	}
	var marked []entity.ChatMessage
	if err = cursor.All(m.ctx, &marked); err != nil {
		return nil, fmt.Errorf("mongodb decode deleted messages: %w", err)
	}
	if len(marked) == 0 {
		return nil, nil
	}

	ids := make(bson.A, len(marked))
	for i := range marked {
		ids[i] = marked[i].ID
	}
	_, err = collection.UpdateMany(m.ctx,
		bson.D{{"_id", bson.D{{"$in", ids}}}},
		bson.D{{"$set", bson.D{{"deleted_at", deletedAt}}}},
	)
	if err != nil {
		return nil, fmt.Errorf("mongodb mark messages deleted: %w", err)
	}
	return marked, nil
}

// SaveHistoryImport records an import of a chat's history; repeated imports add up and
// widen the covered period (zero OldestAt/NewestAt = nothing new was imported).
func (m *MongoDB) SaveHistoryImport(rec entity.ChatHistoryImport) error {
	filter := bson.D{{"platform", rec.Platform}, {"user_id", rec.UserID}, {"channel", rec.Channel}}
	update := bson.D{
		{"$set", bson.D{{"imported_at", rec.ImportedAt}, {"imported_by", rec.ImportedBy}}},
		{"$inc", bson.D{{"count", rec.Count}}},
	}
	if !rec.OldestAt.IsZero() {
		update = append(update,
			bson.E{Key: "$min", Value: bson.D{{"oldest_at", rec.OldestAt}}},
			bson.E{Key: "$max", Value: bson.D{{"newest_at", rec.NewestAt}}},
		)
	}
	_, err := m.collection(historyImportsCollection).UpdateOne(m.ctx, filter, update, options.Update().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("mongodb save history import: %w", err)
	}
	return nil
}

// GetHistoryImport returns the import record of a chat, or nil when none was imported.
func (m *MongoDB) GetHistoryImport(platform, userID, channel string) (*entity.ChatHistoryImport, error) {
	filter := bson.D{{"platform", platform}, {"user_id", userID}, {"channel", channel}}
	var rec entity.ChatHistoryImport
	err := m.collection(historyImportsCollection).FindOne(m.ctx, filter).Decode(&rec)
	if err != nil {
		return nil, m.findError(err)
	}
	return &rec, nil
}

package repository

import (
	"DarkCS/entity"
	"fmt"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Telegram group collections: groups the bot was added to (with their approval status)
// and cached forum topic names.
const (
	groupsCollection      = "tg-groups"
	groupTopicsCollection = "tg-group-topics"
)

// RegisterTelegramGroup records the bot joining a group. A new group is inserted with the
// given (pending) status; a known one only gets its title/rights refreshed and, when it was
// left before, goes back to pending so a re-added group needs approval again. Approved and
// rejected groups keep their status.
func (m *MongoDB) RegisterTelegramGroup(g entity.TelegramGroup) (*entity.TelegramGroup, error) {
	set := bson.D{
		{"title", g.Title},
		{"is_forum", g.IsForum},
		{"can_post", g.CanPost},
		{"updated_at", g.UpdatedAt},
	}
	if g.Username != "" {
		set = append(set, bson.E{Key: "username", Value: g.Username})
	}
	setOnInsert := bson.D{
		{"status", g.Status},
		{"added_by_id", g.AddedByID},
		{"added_by_name", g.AddedByName},
		{"created_at", g.CreatedAt},
	}

	coll := m.collection(groupsCollection)
	_, err := coll.UpdateOne(m.ctx,
		bson.D{{"chat_id", g.ChatID}},
		bson.D{{"$set", set}, {"$setOnInsert", setOnInsert}},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return nil, fmt.Errorf("mongodb register telegram group: %w", err)
	}

	// A group the bot was removed from starts over as pending, with the new adder.
	_, err = coll.UpdateOne(m.ctx,
		bson.D{{"chat_id", g.ChatID}, {"status", entity.GroupStatusLeft}},
		bson.D{{"$set", bson.D{
			{"status", entity.GroupStatusPending},
			{"added_by_id", g.AddedByID},
			{"added_by_name", g.AddedByName},
		}}, {"$unset", bson.D{{"reviewed_by", ""}, {"reviewed_at", ""}}}},
	)
	if err != nil {
		return nil, fmt.Errorf("mongodb reset left telegram group: %w", err)
	}
	return m.GetTelegramGroup(g.ChatID)
}

// updateTelegramGroup sets the given fields of a group (no upsert).
func (m *MongoDB) updateTelegramGroup(chatID int64, fields bson.D) error {
	_, err := m.collection(groupsCollection).UpdateOne(m.ctx,
		bson.D{{"chat_id", chatID}},
		bson.D{{"$set", fields}},
	)
	if err != nil {
		return fmt.Errorf("mongodb update telegram group: %w", err)
	}
	return nil
}

// SetTelegramGroupStatus changes a group's status; reviewer is recorded for approve/reject.
func (m *MongoDB) SetTelegramGroupStatus(chatID int64, status, reviewer string, at time.Time) error {
	fields := bson.D{{"status", status}, {"updated_at", at}}
	if reviewer != "" {
		fields = append(fields, bson.E{Key: "reviewed_by", Value: reviewer}, bson.E{Key: "reviewed_at", Value: at})
	}
	return m.updateTelegramGroup(chatID, fields)
}

// MarkTelegramGroupLeft records the bot being removed from a group. A rejected group
// keeps its status, so adding the bot back makes it leave again instead of asking anew.
func (m *MongoDB) MarkTelegramGroupLeft(chatID int64, at time.Time) error {
	_, err := m.collection(groupsCollection).UpdateOne(m.ctx,
		bson.D{{"chat_id", chatID}, {"status", bson.D{{"$ne", entity.GroupStatusRejected}}}},
		bson.D{{"$set", bson.D{{"status", entity.GroupStatusLeft}, {"can_post", false}, {"updated_at", at}}}},
	)
	if err != nil {
		return fmt.Errorf("mongodb mark telegram group left: %w", err)
	}
	return nil
}

// SetTelegramGroupTitle stores a renamed group's title.
func (m *MongoDB) SetTelegramGroupTitle(chatID int64, title string) error {
	return m.updateTelegramGroup(chatID, bson.D{{"title", title}, {"updated_at", time.Now()}})
}

// GetTelegramGroup returns a group by chat id, or nil when the bot never joined it.
func (m *MongoDB) GetTelegramGroup(chatID int64) (*entity.TelegramGroup, error) {
	var g entity.TelegramGroup
	err := m.collection(groupsCollection).FindOne(m.ctx, bson.D{{"chat_id", chatID}}).Decode(&g)
	if err != nil {
		return nil, m.findError(err)
	}
	return &g, nil
}

// ListTelegramGroups returns groups with the given status (all when empty), newest first.
func (m *MongoDB) ListTelegramGroups(status string) ([]entity.TelegramGroup, error) {
	filter := bson.D{}
	if status != "" {
		filter = bson.D{{"status", status}}
	}
	cursor, err := m.collection(groupsCollection).Find(m.ctx, filter,
		options.Find().SetSort(bson.D{{"created_at", -1}}))
	if err != nil {
		return nil, fmt.Errorf("mongodb find telegram groups: %w", err)
	}
	var groups []entity.TelegramGroup
	if err = cursor.All(m.ctx, &groups); err != nil {
		return nil, fmt.Errorf("mongodb decode telegram groups: %w", err)
	}
	return groups, nil
}

// UpsertGroupTopic stores a topic's name and/or closed flag. An empty name keeps the
// stored one (closed/reopened events carry no name); a nil closed keeps the flag.
func (m *MongoDB) UpsertGroupTopic(chatID, threadID int64, name string, closed *bool) error {
	set := bson.D{{"updated_at", time.Now()}}
	if name != "" {
		set = append(set, bson.E{Key: "name", Value: name})
	}
	if closed != nil {
		set = append(set, bson.E{Key: "closed", Value: *closed})
	}
	_, err := m.collection(groupTopicsCollection).UpdateOne(m.ctx,
		bson.D{{"chat_id", chatID}, {"thread_id", threadID}},
		bson.D{{"$set", set}},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("mongodb upsert group topic: %w", err)
	}
	return nil
}

// NameGroupTopicIfUnknown stores a topic name only when none is stored yet, so a name
// learned from a topic's creation message never overwrites a later rename. A pipeline
// upsert keeps it one atomic write; $literal keeps a name starting with "$" literal.
func (m *MongoDB) NameGroupTopicIfUnknown(chatID, threadID int64, name string) error {
	update := bson.A{bson.D{{"$set", bson.D{
		{"name", bson.D{{"$cond", bson.A{
			bson.D{{"$gt", bson.A{bson.D{{"$ifNull", bson.A{"$name", ""}}}, ""}}},
			"$name",
			bson.D{{"$literal", name}},
		}}}},
		{"updated_at", time.Now()},
	}}}}
	_, err := m.collection(groupTopicsCollection).UpdateOne(m.ctx,
		bson.D{{"chat_id", chatID}, {"thread_id", threadID}},
		update,
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("mongodb name group topic: %w", err)
	}
	return nil
}

// ListGroupTopics returns the known topics of one group, or of every group when chatID is 0.
func (m *MongoDB) ListGroupTopics(chatID int64) ([]entity.TelegramGroupTopic, error) {
	filter := bson.D{}
	if chatID != 0 {
		filter = bson.D{{"chat_id", chatID}}
	}
	cursor, err := m.collection(groupTopicsCollection).Find(m.ctx, filter,
		options.Find().SetSort(bson.D{{"thread_id", 1}}))
	if err != nil {
		return nil, fmt.Errorf("mongodb find group topics: %w", err)
	}
	var topics []entity.TelegramGroupTopic
	if err = cursor.All(m.ctx, &topics); err != nil {
		return nil, fmt.Errorf("mongodb decode group topics: %w", err)
	}
	return topics, nil
}

// MigrateTelegramGroup moves a group upgraded to a supergroup to its new chat id: the group
// record, its topics, stored messages and read receipts. A plain group has no topics, so its
// messages (empty channel) stay one chat. Every step is idempotent, so a retry is safe.
func (m *MongoDB) MigrateTelegramGroup(oldChatID, newChatID int64) error {
	oldID, newID := strconv.FormatInt(oldChatID, 10), strconv.FormatInt(newChatID, 10)

	groups := m.collection(groupsCollection)
	old, err := m.GetTelegramGroup(oldChatID)
	if err != nil {
		return err
	}
	if old != nil {
		// A message of the new supergroup may have registered it (pending) before the
		// migration message arrived; the old record with its review status wins.
		if _, err = groups.DeleteOne(m.ctx, bson.D{{"chat_id", newChatID}}); err != nil {
			return fmt.Errorf("mongodb drop duplicate telegram group: %w", err)
		}
		if _, err = groups.UpdateOne(m.ctx,
			bson.D{{"chat_id", oldChatID}},
			bson.D{{"$set", bson.D{{"chat_id", newChatID}, {"updated_at", time.Now()}}}},
		); err != nil {
			return fmt.Errorf("mongodb migrate telegram group: %w", err)
		}
	}
	if _, err := m.collection(groupTopicsCollection).UpdateMany(m.ctx,
		bson.D{{"chat_id", oldChatID}},
		bson.D{{"$set", bson.D{{"chat_id", newChatID}}}},
	); err != nil {
		return fmt.Errorf("mongodb migrate group topics: %w", err)
	}
	if _, err := m.collection(chatMessagesCollection).UpdateMany(m.ctx,
		bson.D{{"platform", entity.PlatformTelegramGroup}, {"user_id", oldID}},
		bson.D{{"$set", bson.D{{"user_id", newID}, {"chat_id", newID}}}},
	); err != nil {
		return fmt.Errorf("mongodb migrate group messages: %w", err)
	}
	if _, err := m.collection(readReceiptsCollection).UpdateMany(m.ctx,
		bson.D{{"platform", entity.PlatformTelegramGroup}, {"user_id", oldID}},
		bson.D{{"$set", bson.D{{"user_id", newID}}}},
	); err != nil {
		return fmt.Errorf("mongodb migrate group read receipts: %w", err)
	}
	return nil
}

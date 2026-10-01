package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// indexSpec lists the indexes one collection needs.
type indexSpec struct {
	collection string
	models     []mongo.IndexModel
}

// uniqueWhere builds a unique index that only covers documents matching filter. Partial
// indexes let legacy documents with empty identifiers ("", 0, or the old "+" phone
// placeholder) coexist while every real identifier stays unique.
//
// Indexes use MongoDB's default names (e.g. "platform_1_user_id_1") so they match the
// ones created by earlier versions; a different name for the same keys is an error.
func uniqueWhere(keys bson.D, filter bson.M) mongo.IndexModel {
	return mongo.IndexModel{
		Keys:    keys,
		Options: options.Index().SetUnique(true).SetPartialFilterExpression(filter),
	}
}

func plain(keys bson.D) mongo.IndexModel {
	return mongo.IndexModel{Keys: keys}
}

func unique(keys bson.D) mongo.IndexModel {
	return mongo.IndexModel{Keys: keys, Options: options.Index().SetUnique(true)}
}

// indexSpecs is the full index set. Unique indexes double as the guard against
// check-then-insert races (users, api keys, promo codes, chat states).
var indexSpecs = []indexSpec{
	{usersCollection, []mongo.IndexModel{
		uniqueWhere(bson.D{{"uuid", 1}}, bson.M{"uuid": bson.M{"$gt": ""}}),
		// "+" sorts before "+<digit>", so $gt "+" skips empty and placeholder phones.
		uniqueWhere(bson.D{{"phone", 1}}, bson.M{"phone": bson.M{"$gt": "+"}}),
		uniqueWhere(bson.D{{"email", 1}}, bson.M{"email": bson.M{"$gt": ""}}),
		uniqueWhere(bson.D{{"telegram_id", 1}}, bson.M{"telegram_id": bson.M{"$gt": 0}}),
		uniqueWhere(bson.D{{"instagram_id", 1}}, bson.M{"instagram_id": bson.M{"$gt": ""}}),
		plain(bson.D{{"smart_sender_id", 1}}),
	}},
	{chatStatesCollection, []mongo.IndexModel{
		unique(bson.D{{"platform", 1}, {"user_id", 1}}),
		plain(bson.D{{"workflow_id", 1}, {"current_step", 1}}),
	}},
	{apiKeysCollection, []mongo.IndexModel{
		unique(bson.D{{"key", 1}}),
		plain(bson.D{{"username", 1}}),
	}},
	{promoCodesCollection, []mongo.IndexModel{
		unique(bson.D{{"code", 1}}),
	}},
	{basketCollection, []mongo.IndexModel{
		unique(bson.D{{"userUUID", 1}}),
	}},
	{qrStatCollection, []mongo.IndexModel{
		plain(bson.D{{"smart_sender_id", 1}}),
		plain(bson.D{{"platform", 1}, {"user_id", 1}}),
	}},
	// School names are stored in _id (already unique), so only the QR code lookup needs one.
	{schoolsCollection, []mongo.IndexModel{
		plain(bson.D{{"code", 1}, {"active", 1}}),
	}},
	{assistantCollection, []mongo.IndexModel{
		unique(bson.D{{"name", 1}}),
	}},
	{chatMessagesCollection, []mongo.IndexModel{
		plain(bson.D{{"platform", 1}, {"user_id", 1}, {"created_at", -1}}),
		plain(bson.D{{"created_at", -1}}),
		// Telegram message ids dedupe history imported from Telegram Desktop exports.
		uniqueWhere(bson.D{{"platform", 1}, {"channel", 1}, {"user_id", 1}, {"tg_message_id", 1}},
			bson.M{"tg_message_id": bson.M{"$gt": 0}}),
	}},
	{readReceiptsCollection, []mongo.IndexModel{
		unique(bson.D{{"username", 1}, {"platform", 1}, {"user_id", 1}, {"channel", 1}}),
	}},
	{businessConnectionsCollection, []mongo.IndexModel{
		unique(bson.D{{"owner_user_id", 1}}),
		plain(bson.D{{"connection_id", 1}}),
	}},
	{businessContactsCollection, []mongo.IndexModel{
		unique(bson.D{{"user_id", 1}}),
	}},
	{historyImportsCollection, []mongo.IndexModel{
		unique(bson.D{{"platform", 1}, {"user_id", 1}, {"channel", 1}}),
	}},
}

// droppedIndexes are indexes from earlier versions that conflict with the current set.
// The read receipt index without channel would reject a second receipt for the same
// client on another Telegram Business account.
var droppedIndexes = []struct{ collection, name string }{
	{readReceiptsCollection, "username_1_platform_1_user_id_1"},
}

// EnsureIndexes creates every index in indexSpecs. It continues past failures so one bad
// collection doesn't block the rest, and returns them joined. The usual failure is a
// unique index rejected because of existing duplicates: run cmd/dedupe-users, then restart.
func (m *MongoDB) EnsureIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var errs []error
	for _, d := range droppedIndexes {
		_, err := m.collection(d.collection).Indexes().DropOne(ctx, d.name)
		var cmdErr mongo.CommandError
		// 27 = IndexNotFound, 26 = NamespaceNotFound: already dropped or no collection yet.
		if err != nil && !(errors.As(err, &cmdErr) && (cmdErr.Code == 27 || cmdErr.Code == 26)) {
			errs = append(errs, fmt.Errorf("drop %s %s: %w", d.collection, d.name, err))
		}
	}
	for _, spec := range indexSpecs {
		for _, model := range spec.models {
			name, err := m.collection(spec.collection).Indexes().CreateOne(ctx, model)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s %v: %w", spec.collection, model.Keys, err))
				continue
			}
			m.log.Debug("index ensured", slog.String("collection", spec.collection), slog.String("index", name))
		}
	}
	return errors.Join(errs...)
}

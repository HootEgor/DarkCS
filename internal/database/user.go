package repository

import (
	"DarkCS/entity"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"time"
)

// CreateUser inserts a new user document. Callers must check for an existing user first;
// inserting instead of upserting guarantees a lookup miss can never overwrite a real user.
func (m *MongoDB) CreateUser(user *entity.User) error {
	if user.UUID == "" {
		return fmt.Errorf("user uuid is required")
	}
	connection, err := m.connect()
	if err != nil {
		return err
	}
	defer m.disconnect(connection)

	user.LastSeen = time.Now()

	collection := connection.Database(m.database).Collection(usersCollection)
	res, err := collection.InsertOne(m.ctx, user)
	if err != nil {
		return fmt.Errorf("mongodb insert error: %w", err)
	}
	if id, ok := res.InsertedID.(primitive.ObjectID); ok {
		user.ID = id
	}
	return nil
}

// UpdateUserFields sets only the given fields on the user with this UUID (lastSeen is
// always refreshed). Field-level updates keep concurrent writers from reverting each other.
func (m *MongoDB) UpdateUserFields(uuid string, fields map[string]any) error {
	if uuid == "" {
		return fmt.Errorf("user uuid is required")
	}
	connection, err := m.connect()
	if err != nil {
		return err
	}
	defer m.disconnect(connection)

	set := bson.M{entity.UserFieldLastSeen: time.Now()}
	for k, v := range fields {
		set[k] = v
	}

	collection := connection.Database(m.database).Collection(usersCollection)
	res, err := collection.UpdateOne(m.ctx, bson.D{{"uuid", uuid}}, bson.M{"$set": set})
	if err != nil {
		return fmt.Errorf("mongodb update error: %w", err)
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("user %s not found", uuid)
	}
	return nil
}

// SetUserUUID assigns a UUID to a legacy document that was stored without one.
// Matching on _id and an empty uuid makes it safe to race.
func (m *MongoDB) SetUserUUID(id primitive.ObjectID, uuid string) error {
	connection, err := m.connect()
	if err != nil {
		return err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(usersCollection)
	filter := bson.D{{"_id", id}, {"uuid", bson.D{{"$in", bson.A{"", nil}}}}}
	_, err = collection.UpdateOne(m.ctx, filter, bson.M{"$set": bson.M{"uuid": uuid}})
	if err != nil {
		return fmt.Errorf("mongodb update error: %w", err)
	}
	return nil
}

// PushConversation appends one dialog message and keeps only the newest `keep` entries,
// atomically on the server, so it never touches other user fields. A pipeline update is
// used because legacy documents store conversation as null, which $push rejects; $literal
// stops user text starting with "$" from being read as a field path.
func (m *MongoDB) PushConversation(uuid string, message entity.DialogMessage, keep int) error {
	if uuid == "" {
		return fmt.Errorf("user uuid is required")
	}
	connection, err := m.connect()
	if err != nil {
		return err
	}
	defer m.disconnect(connection)

	appended := bson.M{"$concatArrays": bson.A{
		bson.M{"$ifNull": bson.A{"$" + entity.UserFieldConversation, bson.A{}}},
		bson.M{"$literal": bson.A{message}},
	}}
	update := mongo.Pipeline{
		{{"$set", bson.M{
			entity.UserFieldConversation: bson.M{"$slice": bson.A{appended, -keep}},
			entity.UserFieldLastSeen:     time.Now(),
		}}},
	}

	collection := connection.Database(m.database).Collection(usersCollection)
	res, err := collection.UpdateOne(m.ctx, bson.D{{"uuid", uuid}}, update)
	if err != nil {
		return fmt.Errorf("mongodb update error: %w", err)
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("user %s not found", uuid)
	}
	return nil
}

func (m *MongoDB) GetUserByInstagramId(instagramId string) (*entity.User, error) {
	connection, err := m.connect()
	if err != nil {
		return nil, err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(usersCollection)

	filter := bson.D{{"instagram_id", instagramId}}

	var user entity.User
	err = collection.FindOne(m.ctx, filter).Decode(&user)
	if err != nil {
		return nil, m.findError(err)
	}

	return &user, nil
}

func (m *MongoDB) GetUser(email, phone string, telegramId int64) (*entity.User, error) {
	connection, err := m.connect()
	if err != nil {
		return nil, err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(usersCollection)

	// Build dynamic $or filter
	var orFilter []bson.D
	if telegramId != 0 {
		orFilter = append(orFilter, bson.D{{"telegram_id", telegramId}})
	}
	if email != "" {
		orFilter = append(orFilter, bson.D{{"email", email}})
	}
	if phone = entity.NormalizePhone(phone); phone != "" {
		orFilter = append(orFilter, bson.D{{"phone", phone}})
	}

	if len(orFilter) == 0 {
		return nil, fmt.Errorf("no valid identifier fields to search")
	}

	filter := bson.D{{"$or", orFilter}}

	var user entity.User
	err = collection.FindOne(m.ctx, filter).Decode(&user)
	if err != nil {
		return nil, m.findError(err)
	}

	return &user, nil
}

func (m *MongoDB) GetUserBySmartSenderId(smartSenderId string) (*entity.User, error) {
	connection, err := m.connect()
	if err != nil {
		return nil, err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(usersCollection)

	filter := bson.D{{"smart_sender_id", smartSenderId}}

	var user entity.User
	err = collection.FindOne(m.ctx, filter).Decode(&user)
	if err != nil {
		return nil, m.findError(err)
	}

	return &user, nil
}

func (m *MongoDB) GetUserByUUID(uuid string) (*entity.User, error) {
	connection, err := m.connect()
	if err != nil {
		return nil, err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(usersCollection)

	filter := bson.D{{"uuid", uuid}}

	var user entity.User
	err = collection.FindOne(m.ctx, filter).Decode(&user)
	if err != nil {
		return nil, m.findError(err)
	}

	return &user, nil
}

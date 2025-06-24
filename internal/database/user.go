package repository

import (
	"DarkCS/entity"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
	"time"
)

func (m *MongoDB) UpsertUser(user entity.User) error {
	connection, err := m.connect()
	if err != nil {
		return err
	}
	defer m.disconnect(connection)

	user.LastSeen = time.Now()

	collection := connection.Database(m.database).Collection(usersCollection)
	filter := bson.D{{"$or", []bson.D{
		{{"email", user.Email}},
		{{"phone", user.Phone}},
		{{"telegram_id", user.TelegramId}},
	}}}
	update := bson.M{"$set": user}

	_, err = collection.UpdateOne(m.ctx, filter, update, options.Update().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("mongodb upsert error: %w", err)
	}
	return nil
}

func (m *MongoDB) GetUser(email, phone string, telegramId int64) (*entity.User, error) {
	connection, err := m.connect()
	if err != nil {
		return nil, err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(usersCollection)
	filter := bson.D{{"$or", []bson.D{
		{{"email", email}},
		{{"phone", phone}},
		{{"telegram_id", telegramId}},
	}}}

	var user entity.User
	err = collection.FindOne(m.ctx, filter).Decode(&user)
	if err != nil {
		return nil, m.findError(err)
	}

	return &user, nil
}

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

	// Build dynamic $or filter
	var orFilter []bson.D
	if user.TelegramId != 0 {
		orFilter = append(orFilter, bson.D{{"telegram_id", user.TelegramId}})
	}
	if user.Email != "" {
		orFilter = append(orFilter, bson.D{{"email", user.Email}})
	}
	if user.Phone != "" {
		orFilter = append(orFilter, bson.D{{"phone", user.Phone}})
	}

	if len(orFilter) == 0 {
		return fmt.Errorf("no valid identifier fields to upsert")
	}

	filter := bson.D{{"$or", orFilter}}
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

	// Build dynamic $or filter
	var orFilter []bson.D
	if telegramId != 0 {
		orFilter = append(orFilter, bson.D{{"telegram_id", telegramId}})
	}
	if email != "" {
		orFilter = append(orFilter, bson.D{{"email", email}})
	}
	if phone != "" {
		orFilter = append(orFilter, bson.D{{"phone", phone}})
	}

	if len(orFilter) == 0 {
		return nil, fmt.Errorf("no valid identifier fields to upsert")
	}

	filter := bson.D{{"$or", orFilter}}

	var user entity.User
	err = collection.FindOne(m.ctx, filter).Decode(&user)
	if err != nil {
		return nil, m.findError(err)
	}

	return &user, nil
}

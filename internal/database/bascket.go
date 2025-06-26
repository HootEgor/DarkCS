package repository

import (
	"DarkCS/entity"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (m *MongoDB) UpsertBasket(basket *entity.Basket) (*entity.Basket, error) {
	connection, err := m.connect()
	if err != nil {
		return nil, err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(basketCollection)

	filter := bson.D{{"userUUID", basket.UserUUID}}
	update := bson.M{"$set": basket}

	_, err = collection.UpdateOne(m.ctx, filter, update, options.Update().SetUpsert(true))
	if err != nil {
		return nil, fmt.Errorf("mongodb upsert error: %w", err)
	}
	return basket, nil
}

func (m *MongoDB) GetBasket(userUUID string) (*entity.Basket, error) {
	connection, err := m.connect()
	if err != nil {
		return nil, err
	}
	defer m.disconnect(connection)

	filter := bson.D{{"userUUID", userUUID}}
	collection := connection.Database(m.database).Collection(basketCollection)
	result := collection.FindOne(m.ctx, filter)
	if result.Err() != nil {
		return nil, m.findError(result.Err())
	}
	basket := &entity.Basket{}
	err = result.Decode(basket)
	if err != nil {
		return nil, fmt.Errorf("mongodb decode error: %w", err)
	}
	return basket, nil
}

package repository

import (
	"DarkCS/entity"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (m *MongoDB) UpsertAssistant(assistant *entity.Assistant) (*entity.Assistant, error) {
	connection, err := m.connect()
	if err != nil {
		return nil, err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(assistantCollection)

	filter := bson.D{{"name", assistant.Name}}
	update := bson.D{{"$set", assistant}}

	opts := options.Update().SetUpsert(true)
	result, err := collection.UpdateOne(m.ctx, filter, update, opts)
	if err != nil {
		return nil, fmt.Errorf("mongodb upsert assistant: %w", err)
	}

	if result.MatchedCount == 0 && result.UpsertedCount == 0 {
		return nil, fmt.Errorf("no documents matched for upsert")
	}

	return assistant, nil
}

func (m *MongoDB) GetAssistant(name string) (*entity.Assistant, error) {
	connection, err := m.connect()
	if err != nil {
		return nil, err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(assistantCollection)

	filter := bson.D{{"name", name}}
	var assistant entity.Assistant
	err = collection.FindOne(m.ctx, filter).Decode(&assistant)
	if err != nil {
		if err.Error() == "mongo: no documents in result" {
			return nil, fmt.Errorf("assistant with name %s not found", name)
		}
		return nil, fmt.Errorf("mongodb find assistant: %w", err)
	}
	if assistant.Name == "" {
		return nil, fmt.Errorf("assistant not found")
	}
	return &assistant, nil
}

func (m *MongoDB) GetAllAssistants() ([]entity.Assistant, error) {
	connection, err := m.connect()
	if err != nil {
		return nil, err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(assistantCollection)

	cursor, err := collection.Find(m.ctx, bson.D{})
	if err != nil {
		return nil, fmt.Errorf("mongodb find assistants: %w", err)
	}
	defer cursor.Close(m.ctx)

	var assistants []entity.Assistant
	if err = cursor.All(m.ctx, &assistants); err != nil {
		return nil, fmt.Errorf("mongodb decode assistants: %w", err)
	}

	return assistants, nil
}

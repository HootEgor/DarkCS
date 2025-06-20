package repository

import (
	"DarkCS/entity"
	"fmt"
)

func (m *MongoDB) SaveMessage(message entity.Message) error {
	connection, err := m.connect()
	if err != nil {
		return err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(messagesCollection)
	_, err = collection.InsertOne(m.ctx, message)
	if err != nil {
		return fmt.Errorf("mongodb insert message: %w", err)
	}
	return nil
}

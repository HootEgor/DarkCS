package repository

import (
	"DarkCS/entity"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
	"time"
)

func (m *MongoDB) FollowQr(smartSenderId string) error {
	connection, err := m.connect()
	if err != nil {
		return err
	}
	defer m.disconnect(connection)

	qrStat := entity.QrStat{
		SmartSenderId: smartSenderId,
		Date:          time.Now(),
		FollowQr:      true,
		Registered:    false,
	}

	collection := connection.Database(m.database).Collection(qrStatCollection)

	filter := bson.D{{"smart_sender_id", smartSenderId}}
	update := bson.M{"$set": qrStat}

	_, err = collection.UpdateOne(m.ctx, filter, update, options.Update().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("mongodb upsert error: %w", err)
	}
	return nil
}

func (m *MongoDB) RegisterQr(smartSenderId string) error {
	connection, err := m.connect()
	if err != nil {
		return err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(qrStatCollection)

	filter := bson.D{{"smart_sender_id", smartSenderId}}
	update := bson.M{"$set": bson.M{"registered": true}}

	_, err = collection.UpdateOne(m.ctx, filter, update, options.Update())
	if err != nil {
		return fmt.Errorf("mongodb update error: %w", err)
	}
	return nil
}

// SaveSchoolStat records or updates the school selected by a user after onboarding.
// Keyed by platform + user_id since SmartSender is deprecated.
func (m *MongoDB) SaveSchoolStat(platform, userID, schoolName string) error {
	connection, err := m.connect()
	if err != nil {
		return err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(qrStatCollection)

	filter := bson.D{{"platform", platform}, {"user_id", userID}}
	update := bson.M{
		"$set": bson.M{
			"platform":    platform,
			"user_id":     userID,
			"school_name": schoolName,
			"date":        time.Now(),
			"follow_qr":   true,
			"registered":  true,
		},
	}

	_, err = collection.UpdateOne(m.ctx, filter, update, options.Update().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("mongodb upsert school stat: %w", err)
	}
	return nil
}

func (m *MongoDB) GetAllQrStat() ([]entity.QrStat, error) {
	connection, err := m.connect()
	if err != nil {
		return nil, err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(qrStatCollection)

	cursor, err := collection.Find(m.ctx, bson.D{})
	if err != nil {
		return nil, fmt.Errorf("mongodb find qrstat: %w", err)
	}
	defer cursor.Close(m.ctx)

	var stat []entity.QrStat
	if err = cursor.All(m.ctx, &stat); err != nil {
		return nil, fmt.Errorf("mongodb decode assistants: %w", err)
	}

	return stat, nil
}

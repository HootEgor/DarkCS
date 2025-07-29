package repository

import (
	"DarkCS/entity"
	"errors"
	"fmt"
	"go.mongodb.org/mongo-driver/mongo"
)

func (m *MongoDB) SavePromoCodes(codes []string) error {
	connection, err := m.connect()
	if err != nil {
		return err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(promoCodesCollection)

	docs := make([]interface{}, len(codes))
	for i, code := range codes {
		docs[i] = entity.PromoCode{
			Code:      code,
			Activated: false,
		}
	}

	_, err = collection.InsertMany(m.ctx, docs)
	if err != nil {
		return fmt.Errorf("mongodb insert promoCodes: %w", err)
	}

	return nil
}

func (m *MongoDB) GetAllPromoCodes() ([]entity.PromoCode, error) {
	connection, err := m.connect()
	if err != nil {
		return nil, err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(promoCodesCollection)

	cursor, err := collection.Find(m.ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("mongodb find promoCodes: %w", err)
	}
	defer cursor.Close(m.ctx)

	var promoCodes []entity.PromoCode
	if err = cursor.All(m.ctx, &promoCodes); err != nil {
		return nil, fmt.Errorf("mongodb decode promoCodes: %w", err)
	}

	return promoCodes, nil
}

func (m *MongoDB) ActivatePromoCode(code string) error {
	connection, err := m.connect()
	if err != nil {
		return err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(promoCodesCollection)

	filter := entity.PromoCode{Code: code, Activated: false}
	update := entity.PromoCode{Activated: true}

	result, err := collection.UpdateOne(m.ctx, filter, update)
	if err != nil {
		return fmt.Errorf("mongodb update promoCode: %w", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("promo code %s not found or already activated", code)
	}

	return nil
}

func (m *MongoDB) GetPromoCode(code string) (*entity.PromoCode, error) {
	connection, err := m.connect()
	if err != nil {
		return nil, err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(promoCodesCollection)

	filter := entity.PromoCode{Code: code}
	var promoCode entity.PromoCode
	err = collection.FindOne(m.ctx, filter).Decode(&promoCode)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil // Promo code not found
		}
		return nil, fmt.Errorf("mongodb find promoCode: %w", err)
	}

	return &promoCode, nil
}

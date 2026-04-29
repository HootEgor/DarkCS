package repository

import (
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
	"time"
)

const instagramTokenCollection = "instagram_token"

// SaveInstagramToken upserts the long-lived access token as a singleton document.
// Called after each successful token refresh so the current token survives restarts
// and can be read by other services (e.g. DarkBot) on the same MongoDB server.
func (m *MongoDB) SaveInstagramToken(token string) error {
	conn, err := m.connect()
	if err != nil {
		return err
	}
	defer m.disconnect(conn)

	_, err = conn.Database(m.database).Collection(instagramTokenCollection).UpdateOne(
		m.ctx,
		bson.M{"_id": "singleton"},
		bson.M{"$set": bson.M{
			"access_token": token,
			"updated_at":   time.Now(),
		}},
		options.Update().SetUpsert(true),
	)
	return err
}

// GetInstagramToken returns the persisted long-lived access token.
// Returns an empty string (no error) when no token has been saved yet.
func (m *MongoDB) GetInstagramToken() (string, error) {
	conn, err := m.connect()
	if err != nil {
		return "", err
	}
	defer m.disconnect(conn)

	var doc struct {
		AccessToken string `bson:"access_token"`
	}
	err = conn.Database(m.database).Collection(instagramTokenCollection).
		FindOne(m.ctx, bson.M{"_id": "singleton"}).
		Decode(&doc)
	if err != nil {
		if e := m.findError(err); e == nil {
			return "", nil
		}
		return "", err
	}
	return doc.AccessToken, nil
}

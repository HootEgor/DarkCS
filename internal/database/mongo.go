package repository

import (
	"DarkCS/internal/config"
	"DarkCS/internal/lib/sl"
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	_ "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"log/slog"
)

const (
	usersCollection      = "users"
	basketCollection     = "baskets"
	messagesCollection   = "messages"
	apiKeysCollection    = "api-keys"
	promoCodesCollection = "promo-codes"
	assistantCollection  = "assistant"
	qrStatCollection     = "qr-stat"
)

type MongoDB struct {
	ctx           context.Context
	clientOptions *options.ClientOptions
	database      string
	log           *slog.Logger
}

func NewMongoClient(conf *config.Config, logger *slog.Logger) (*MongoDB, error) {
	if !conf.Mongo.Enabled {
		return nil, nil
	}
	connectionUri := fmt.Sprintf("mongodb://%s:%s", conf.Mongo.Host, conf.Mongo.Port)
	clientOptions := options.Client().ApplyURI(connectionUri)
	if conf.Mongo.User != "" {
		clientOptions.SetAuth(options.Credential{
			Username:   conf.Mongo.User,
			Password:   conf.Mongo.Password,
			AuthSource: conf.Mongo.Database,
		})
	}
	client := &MongoDB{
		ctx:           context.Background(),
		clientOptions: clientOptions,
		database:      conf.Mongo.Database,
		log:           logger.With(sl.Module("mongodb")),
	}
	return client, nil
}

func (m *MongoDB) connect() (*mongo.Client, error) {
	connection, err := mongo.Connect(m.ctx, m.clientOptions)
	if err != nil {
		return nil, fmt.Errorf("mongodb connect error: %w", err)
	}
	return connection, nil
}

func (m *MongoDB) disconnect(connection *mongo.Client) {
	_ = connection.Disconnect(m.ctx)
}

func (m *MongoDB) findError(err error) error {
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil
	}
	return fmt.Errorf("mongodb find error: %w", err)
}

func (m *MongoDB) CheckApiKey(key string) (string, error) {
	connection, err := m.connect()
	if err != nil {
		return "", err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(apiKeysCollection)
	filter := bson.D{{"key", key}}

	var result struct {
		Username string `bson:"username"`
		Kay      string `bson:"key"`
	}
	err = collection.FindOne(m.ctx, filter).Decode(&result)
	if err != nil {
		return "", err
	}

	if result.Username == "" {
		return "", fmt.Errorf("api key not found")
	}

	return result.Username, nil
}

func (m *MongoDB) getKeyByUsername(username string) (string, error) {
	connection, err := m.connect()
	if err != nil {
		return "", err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(apiKeysCollection)
	filter := bson.D{{"username", username}}

	var result struct {
		Key string `bson:"key"`
	}
	err = collection.FindOne(m.ctx, filter).Decode(&result)
	if err != nil {
		return "", m.findError(err)
	}

	return result.Key, nil
}

func (m *MongoDB) GenerateApiKey(username string) (string, error) {

	k, err := m.getKeyByUsername(username)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return "", fmt.Errorf("failed to get existing API key: %w", err)
	}
	if k != "" {
		return k, nil
	}

	connection, err := m.connect()
	if err != nil {
		return "", err
	}
	defer m.disconnect(connection)

	collection := connection.Database(m.database).Collection(apiKeysCollection)
	uuid, err := uuid.NewUUID()
	if err != nil {
		return "", fmt.Errorf("uuid generation error: %w", err)
	}
	key := uuid.String()

	doc := bson.D{
		{"username", username},
		{"key", key},
	}

	_, err = collection.InsertOne(m.ctx, doc)
	if err != nil {
		return "", fmt.Errorf("mongodb insert error: %w", err)
	}

	return key, nil
}

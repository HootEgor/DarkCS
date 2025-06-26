package repository

import (
	"DarkCS/internal/config"
	"DarkCS/internal/lib/sl"
	"context"
	"errors"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	_ "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"log/slog"
)

const (
	usersCollection    = "users"
	basketCollection   = "baskets"
	messagesCollection = "messages"
	apiKeysCollection  = "api-keys"
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

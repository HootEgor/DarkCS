package repository

import (
	"DarkCS/internal/config"
	"DarkCS/internal/lib/sl"
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"log/slog"
	"time"
)

const (
	usersCollection        = "users"
	basketCollection       = "baskets"
	messagesCollection     = "messages"
	apiKeysCollection      = "api-keys"
	promoCodesCollection   = "promo-codes"
	assistantCollection    = "assistant"
	qrStatCollection       = "qr-stat"
	chatMessagesCollection = "chat-messages"
	readReceiptsCollection = "chat-read-receipts"
)

// MongoDB is the repository over a single shared, pooled mongo.Client.
//
// Historically every method called connect()/disconnect() around each operation, which
// built a new pool, TCP connection and SCRAM handshake per query. connect() now hands
// out the shared client and disconnect() is a no-op, so existing methods keep their
// shape while reusing pooled connections. New code should use m.collection().
type MongoDB struct {
	ctx      context.Context
	client   *mongo.Client
	database string
	log      *slog.Logger
}

// Client-level limits. No client-wide operation Timeout is set because it would also cap
// long GridFS downloads; SocketTimeout bounds a hung server read instead.
const (
	mongoMaxPoolSize            = 50
	mongoServerSelectionTimeout = 5 * time.Second
	mongoConnectTimeout         = 10 * time.Second
	mongoSocketTimeout          = 30 * time.Second
	mongoStartupPingTimeout     = 10 * time.Second
)

// NewMongoClient connects once and pings the server, so a misconfigured or unreachable
// database fails startup instead of failing every later request.
// Returns (nil, nil) when mongo is disabled in config.
func NewMongoClient(conf *config.Config, logger *slog.Logger) (*MongoDB, error) {
	if !conf.Mongo.Enabled {
		return nil, nil
	}
	connectionUri := fmt.Sprintf("mongodb://%s:%s", conf.Mongo.Host, conf.Mongo.Port)
	clientOptions := options.Client().
		ApplyURI(connectionUri).
		SetMaxPoolSize(mongoMaxPoolSize).
		SetServerSelectionTimeout(mongoServerSelectionTimeout).
		SetConnectTimeout(mongoConnectTimeout).
		SetSocketTimeout(mongoSocketTimeout)
	if conf.Mongo.User != "" {
		clientOptions.SetAuth(options.Credential{
			Username:   conf.Mongo.User,
			Password:   conf.Mongo.Password,
			AuthSource: conf.Mongo.Database,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), mongoStartupPingTimeout)
	defer cancel()

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("mongodb connect error: %w", err)
	}
	if err = client.Ping(ctx, readpref.Primary()); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("mongodb ping error: %w", err)
	}

	return &MongoDB{
		ctx:      context.Background(),
		client:   client,
		database: conf.Mongo.Database,
		log:      logger.With(sl.Module("mongodb")),
	}, nil
}

// Close releases the connection pool; call once on shutdown.
func (m *MongoDB) Close(ctx context.Context) error {
	return m.client.Disconnect(ctx)
}

// collection returns a handle on the shared client.
func (m *MongoDB) collection(name string) *mongo.Collection {
	return m.client.Database(m.database).Collection(name)
}

// connect returns the shared client. Kept so existing repository methods need no change.
func (m *MongoDB) connect() (*mongo.Client, error) {
	return m.client, nil
}

// disconnect is a no-op: the shared client lives for the whole process (see Close).
func (m *MongoDB) disconnect(*mongo.Client) {}

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

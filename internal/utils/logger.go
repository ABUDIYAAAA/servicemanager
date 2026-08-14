package utils

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type LogBroadcaster struct {
	mu        sync.Mutex
	listeners map[chan string]bool
}

var Broadcaster = &LogBroadcaster{
	listeners: make(map[chan string]bool),
}

func (b *LogBroadcaster) Register(ch chan string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.listeners[ch] = true
}

func (b *LogBroadcaster) Unregister(ch chan string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.listeners, ch)
	close(ch)
}

func (b *LogBroadcaster) Broadcast(msg string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.listeners {
		select {
		case ch <- msg:
		default:
			// listener slow or full, skip to avoid blocking logging
		}
	}
}

type MongoHandler struct {
	parent     slog.Handler
	collection *mongo.Collection
}

func (h *MongoHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.parent.Enabled(ctx, level)
}

func (h *MongoHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &MongoHandler{
		parent:     h.parent.WithAttrs(attrs),
		collection: h.collection,
	}
}

func (h *MongoHandler) WithGroup(name string) slog.Handler {
	return &MongoHandler{
		parent:     h.parent.WithGroup(name),
		collection: h.collection,
	}
}

func (h *MongoHandler) Handle(ctx context.Context, r slog.Record) error {
	err := h.parent.Handle(ctx, r)

	fields := make(map[string]any)
	r.Attrs(func(a slog.Attr) bool {
		fields[a.Key] = a.Value.Any()
		return true
	})

	logEntry := bson.M{
		"time":    r.Time,
		"level":   r.Level.String(),
		"message": r.Message,
		"fields":  fields,
	}

	if h.collection != nil {
		go func() {
			dbCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_, _ = h.collection.InsertOne(dbCtx, logEntry)
		}()
	}

	jsonData, encErr := json.Marshal(logEntry)
	if encErr == nil {
		Broadcaster.Broadcast(string(jsonData))
	}

	return err
}

var MongoClient *mongo.Client

// ConnectMongo initializes connection to MongoDB
func ConnectMongo(ctx context.Context, uri string) (*mongo.Client, error) {
	clientOptions := options.Client().ApplyURI(uri)
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, err
	}

	err = client.Ping(ctx, nil)
	if err != nil {
		return nil, err
	}

	MongoClient = client
	return client, nil
}

// InitLogger sets up a structured JSON logger as the default logger, wrapping with MongoHandler if connected.
func InitLogger() {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}

	var handler slog.Handler
	if os.Getenv("ENV") == "development" || os.Getenv("ENV") == "" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	if MongoClient != nil {
		logsCol := MongoClient.Database("servicesmanager").Collection("logs")
		handler = &MongoHandler{
			parent:     handler,
			collection: logsCol,
		}
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
}

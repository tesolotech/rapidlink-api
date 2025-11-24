package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type DatabaseConfig struct {
	Client     *mongo.Client
	Database   *mongo.Database
	Collection *mongo.Collection
}

// DatabaseCollections provides logical separation of collections
type DatabaseCollections struct {
	Users *mongo.Collection
	URLs  *mongo.Collection
}

var DB *DatabaseConfig

// GetCollections returns organized collection references
func GetCollections() *DatabaseCollections {
	return &DatabaseCollections{
		Users: DB.Database.Collection("users"),
		URLs:  DB.Database.Collection("urls"),
	}
}

// InitializeDatabase initializes MongoDB connection with default configuration
func InitializeDatabase() error {
	// Get connection string from environment or use default
	connectionString := os.Getenv("MONGODB_URI")
	if connectionString == "" {
		connectionString = "mongodb://localhost:27017"
	}

	// Get database name from environment or use default
	databaseName := os.Getenv("MONGODB_DATABASE")
	if databaseName == "" {
		databaseName = "url_shortener"
	}

	log.Println("Attempting to connect to MongoDB...")
	log.Printf("Connection String: %s", connectionString)
	log.Printf("Database Name: %s", databaseName)

	if err := InitMongoDB(connectionString, databaseName); err != nil {
		log.Printf("⚠️  MongoDB connection failed: %v", err)
		log.Println("💡 To fix this:")
		log.Println("   1. Install MongoDB: https://www.mongodb.com/try/download/community")
		log.Println("   2. Start MongoDB service:")
		log.Println("      Windows: net start MongoDB")
		log.Println("      Linux/Mac: sudo systemctl start mongod")
		log.Println("   3. Or use Docker: docker run -d -p 27017:27017 --name mongodb mongo:latest")
		log.Println("   4. Set environment variables:")
		log.Println("      export MONGODB_URI=\"mongodb://localhost:27017\"")
		log.Println("      export MONGODB_DATABASE=\"url_shortener\"")
		log.Println("🔄 Starting in demo mode without database...")
		return nil // Allow startup without database for testing
	}

	log.Println("✅ MongoDB connected successfully!")
	return nil
}

// InitMongoDB initializes the MongoDB connection and creates indexes
func InitMongoDB(connectionString, databaseName string) error {
	// Optimize connection pool settings
	clientOptions := options.Client().ApplyURI(connectionString).
		SetMaxPoolSize(100).                       // Max 100 connections in pool
		SetMinPoolSize(10).                        // Min 10 connections always available
		SetMaxConnIdleTime(30 * time.Second).      // Close idle connections after 30s
		SetRetryWrites(true).                      // Auto-retry write operations
		SetRetryReads(true).                       // Auto-retry read operations
		SetConnectTimeout(10 * time.Second).       // 10s connection timeout
		SetServerSelectionTimeout(5 * time.Second) // 5s server selection timeout

	// Connect to MongoDB
	client, err := mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		return fmt.Errorf("failed to connect to MongoDB: %v", err)
	}

	// Check the connection
	err = client.Ping(context.TODO(), nil)
	if err != nil {
		return fmt.Errorf("failed to ping MongoDB: %v", err)
	}

	database := client.Database(databaseName)
	collection := database.Collection("urls")

	DB = &DatabaseConfig{
		Client:     client,
		Database:   database,
		Collection: collection,
	}

	log.Println("Connected to MongoDB!")

	// Create indexes
	if err := createIndexes(); err != nil {
		return fmt.Errorf("failed to create indexes: %v", err)
	}

	log.Println("MongoDB indexes created successfully!")
	return nil
}

// createIndexes creates all necessary indexes for the URLs collection
func createIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Unique index on short_url
	shortURLIndex := mongo.IndexModel{
		Keys:    bson.D{{Key: "short_url", Value: 1}},
		Options: options.Index().SetUnique(true),
	}

	// 2. Partial unique index on long_url (only for active URLs)
	longURLIndex := mongo.IndexModel{
		Keys: bson.D{{Key: "long_url", Value: 1}},
		Options: options.Index().
			SetUnique(true).
			SetPartialFilterExpression(bson.D{{Key: "is_active", Value: true}}),
	}

	// 3. Index on expires_at for cleanup operations
	expiryIndex := mongo.IndexModel{
		Keys:    bson.D{{Key: "expires_at", Value: 1}},
		Options: options.Index().SetSparse(true),
	}

	// 4. Index on created_at for analytics
	createdAtIndex := mongo.IndexModel{
		Keys: bson.D{{Key: "created_at", Value: -1}},
	}

	// 5. Compound index on is_active and created_at
	compoundIndex := mongo.IndexModel{
		Keys: bson.D{
			{Key: "is_active", Value: 1},
			{Key: "created_at", Value: -1},
		},
	}

	// 6. Index on user_id for user-specific queries
	userIndex := mongo.IndexModel{
		Keys: bson.D{{Key: "user_id", Value: 1}},
	}

	// 7. Compound index on user_id and created_at
	userCompoundIndex := mongo.IndexModel{
		Keys: bson.D{
			{Key: "user_id", Value: 1},
			{Key: "created_at", Value: -1},
		},
	}

	// Enhanced indexes for users collection
	userUsernameIndex := mongo.IndexModel{
		Keys:    bson.D{{Key: "username", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("username_unique_idx"),
	}

	userEmailIndex := mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("email_unique_idx"),
	}

	// Compound index for login queries (username/email + active status)
	userLoginIndex := mongo.IndexModel{
		Keys: bson.D{
			{Key: "username", Value: 1},
			{Key: "is_active", Value: 1},
		},
		Options: options.Index().SetName("username_active_idx"),
	}

	// Compound index for email login queries
	userEmailLoginIndex := mongo.IndexModel{
		Keys: bson.D{
			{Key: "email", Value: 1},
			{Key: "is_active", Value: 1},
		},
		Options: options.Index().SetName("email_active_idx"),
	}

	// Index on created_at for user analytics
	userCreatedAtIndex := mongo.IndexModel{
		Keys:    bson.D{{Key: "created_at", Value: -1}},
		Options: options.Index().SetName("user_created_at_idx"),
	}

	// Create all indexes for urls collection
	urlIndexes := []mongo.IndexModel{
		shortURLIndex,
		longURLIndex,
		expiryIndex,
		createdAtIndex,
		compoundIndex,
		userIndex,
		userCompoundIndex,
	}

	_, err := DB.Collection.Indexes().CreateMany(ctx, urlIndexes)
	if err != nil {
		return err
	}

	// Create all enhanced indexes for users collection
	userIndexes := []mongo.IndexModel{
		userUsernameIndex,
		userEmailIndex,
		userLoginIndex,
		userEmailLoginIndex,
		userCreatedAtIndex,
	}

	_, err = DB.Database.Collection("users").Indexes().CreateMany(ctx, userIndexes)
	return err
}

// CleanupExpiredURLs marks expired URLs as inactive
func CleanupExpiredURLs() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	filter := bson.D{
		{Key: "expires_at", Value: bson.D{{Key: "$lte", Value: time.Now()}}},
		{Key: "is_active", Value: true},
	}

	update := bson.D{
		{Key: "$set", Value: bson.D{{Key: "is_active", Value: false}}},
	}

	result, err := DB.Collection.UpdateMany(ctx, filter, update)
	if err != nil {
		return err
	}

	if result.ModifiedCount > 0 {
		log.Printf("Marked %d expired URLs as inactive", result.ModifiedCount)
	}

	return nil
}

// GetDatabaseStats returns collection statistics
func GetDatabaseStats() (bson.M, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var result bson.M
	err := DB.Database.RunCommand(ctx, bson.D{
		{Key: "collStats", Value: "urls"},
	}).Decode(&result)

	return result, err
}

// CloseMongoDB closes the MongoDB connection
func CloseMongoDB() error {
	if DB != nil && DB.Client != nil {
		log.Println("🔌 Closing MongoDB connection...")
		return DB.Client.Disconnect(context.TODO())
	}
	return nil
}

// GetUserURLsPaginated retrieves paginated URLs for a user using skip/limit
func GetUserURLsPaginated(userID string, skip int, limit int) ([]map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if limit <= 0 || limit > 100 {
		limit = 50 // Default limit
	}
	if skip < 0 {
		skip = 0
	}

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.D{{Key: "user_id", Value: userID}, {Key: "is_active", Value: true}}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "created_at", Value: -1}}}},
		bson.D{{Key: "$skip", Value: skip}},
		bson.D{{Key: "$limit", Value: limit}},
		bson.D{{Key: "$project", Value: bson.D{
			{Key: "short_url", Value: 1},
			{Key: "long_url", Value: 1},
			{Key: "domain", Value: 1},
			{Key: "tags", Value: 1},
			{Key: "clicks", Value: 1},
			{Key: "created_at", Value: 1},
			{Key: "expires_at", Value: 1},
			{Key: "is_active", Value: 1},
			{Key: "_id", Value: 0},
		}}},
	}

	cursor, err := DB.Collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var urls []map[string]interface{}
	if err = cursor.All(ctx, &urls); err != nil {
		return nil, err
	}
	return urls, nil
}

// GetUserURLsOptimized retrieves URLs for a user using optimized aggregation
func GetUserURLsOptimized(userID string, limit int) ([]map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if limit <= 0 || limit > 100 {
		limit = 50 // Default limit
	}

	// Optimized aggregation pipeline
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.D{{Key: "user_id", Value: userID}, {Key: "is_active", Value: true}}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "created_at", Value: -1}}}},
		bson.D{{Key: "$limit", Value: limit}},
		bson.D{{Key: "$project", Value: bson.D{
			{Key: "short_url", Value: 1},
			{Key: "long_url", Value: 1},
			{Key: "domain", Value: 1},
			{Key: "tags", Value: 1},
			{Key: "clicks", Value: 1},
			{Key: "created_at", Value: 1},
			{Key: "expires_at", Value: 1},
			{Key: "is_active", Value: 1},
			{Key: "_id", Value: 0},
		}}},
	}

	cursor, err := DB.Collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("aggregation failed: %v", err)
	}
	defer cursor.Close(ctx)

	var urls []map[string]interface{}
	if err = cursor.All(ctx, &urls); err != nil {
		return nil, fmt.Errorf("cursor processing failed: %v", err)
	}

	// Prepare URLs with full BASE_URL for frontend
	// baseURL := os.Getenv("BASE_URL")
	// if baseURL == "" {
	// 	baseURL = "http://localhost:8080" // Default base URL
	// }

	// for i := range urls {
	// 	if shortURL, ok := urls[i]["short_url"].(string); ok {
	// 		urls[i]["short_url"] = baseURL + "/" + shortURL
	// 	}
	// }
	return urls, nil
}

// GetUserStatsOptimized gets user statistics using aggregation
func GetUserStatsOptimized(userID string) (map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stats := map[string]interface{}{
		"total_urls":          0,
		"total_clicks":        0,
		"avg_clicks_per_url":  0,
		"clicks_over_time":    []map[string]interface{}{},
		"tag_distribution":    []map[string]interface{}{},
		"domain_distribution": []map[string]interface{}{},
		"top_links":           []map[string]interface{}{},
	}

	type result struct {
		key   string
		value interface{}
		err   error
	}

	var wg sync.WaitGroup
	ch := make(chan result, 5)

	wg.Add(5)
	go func() {
		defer wg.Done()
		val, err := getBasicStats(ctx, userID)
		ch <- result{"basic", val, err}
	}()
	go func() {
		defer wg.Done()
		val, err := getClicksOverTime(ctx, userID)
		ch <- result{"clicks_over_time", val, err}
	}()
	go func() {
		defer wg.Done()
		val, err := getTagDistribution(ctx, userID)
		ch <- result{"tag_distribution", val, err}
	}()
	go func() {
		defer wg.Done()
		val, err := getDomainDistribution(ctx, userID)
		ch <- result{"domain_distribution", val, err}
	}()
	go func() {
		defer wg.Done()
		val, err := getTopLinks(ctx, userID)
		ch <- result{"top_links", val, err}
	}()

	wg.Wait()
	close(ch)

	for res := range ch {
		if res.err != nil {
			if res.key == "basic" {
				return nil, res.err
			}
			log.Printf("Warning: analytics aggregation for %s failed: %v", res.key, res.err)
			continue
		}
		switch res.key {
		case "basic":
			if res.value != nil {
				for k, v := range res.value.(map[string]interface{}) {
					stats[k] = v
				}
			}
		default:
			stats[res.key] = res.value
		}
	}

	return stats, nil
}

// Helper functions for GetUserStatsOptimized

func getBasicStats(ctx context.Context, userID string) (map[string]interface{}, error) {
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.D{{Key: "user_id", Value: userID}, {Key: "is_active", Value: true}}}},
		bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: nil},
			{Key: "total_urls", Value: bson.D{{Key: "$sum", Value: 1}}},
			{Key: "total_clicks", Value: bson.D{{Key: "$sum", Value: "$clicks"}}},
			{Key: "avg_clicks_per_url", Value: bson.D{{Key: "$avg", Value: "$clicks"}}},
		}}},
		bson.D{{Key: "$project", Value: bson.D{
			{Key: "_id", Value: 0},
			{Key: "total_urls", Value: 1},
			{Key: "total_clicks", Value: 1},
			{Key: "avg_clicks_per_url", Value: bson.D{{Key: "$round", Value: bson.A{"$avg_clicks_per_url", 2}}}},
		}}},
	}
	cursor, err := DB.Collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var results []map[string]interface{}
	if err = cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	if len(results) > 0 {
		return results[0], nil
	}
	return nil, nil
}

func getClicksOverTime(ctx context.Context, userID string) ([]map[string]interface{}, error) {
	clicksOverTime := []map[string]interface{}{}
	clicksPipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.D{
			{Key: "user_id", Value: userID},
			{Key: "is_active", Value: true},
			{Key: "created_at", Value: bson.D{{Key: "$gte", Value: time.Now().AddDate(0, 0, -30)}}},
		}}},
		bson.D{{Key: "$unwind", Value: "$click_history"}},
		bson.D{{Key: "$match", Value: bson.D{
			{Key: "click_history.timestamp", Value: bson.D{{Key: "$gte", Value: time.Now().AddDate(0, 0, -30)}}},
		}}},
		bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: bson.D{
				{Key: "$dateToString", Value: bson.D{
					{Key: "format", Value: "%Y-%m-%d"},
					{Key: "date", Value: "$click_history.timestamp"},
				}},
			}},
			{Key: "clicks", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
	}
	clickCursor, err := DB.Collection.Aggregate(ctx, clicksPipeline)
	if err != nil {
		return clicksOverTime, nil
	}
	defer clickCursor.Close(ctx)
	for clickCursor.Next(ctx) {
		var doc map[string]interface{}
		if err := clickCursor.Decode(&doc); err == nil {
			clicksOverTime = append(clicksOverTime, map[string]interface{}{
				"date":   doc["_id"],
				"clicks": doc["clicks"],
			})
		}
	}
	return clicksOverTime, nil
}

func getTagDistribution(ctx context.Context, userID string) ([]map[string]interface{}, error) {
	tagDistribution := []map[string]interface{}{}
	tagPipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.D{
			{Key: "user_id", Value: userID},
			{Key: "is_active", Value: true},
		}}},
		bson.D{{Key: "$unwind", Value: "$tags"}},
		bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$tags"},
			{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}}}},
		bson.D{{Key: "$limit", Value: 10}},
	}
	tagCursor, err := DB.Collection.Aggregate(ctx, tagPipeline)
	if err != nil {
		return tagDistribution, nil
	}
	defer tagCursor.Close(ctx)
	for tagCursor.Next(ctx) {
		var doc map[string]interface{}
		if err := tagCursor.Decode(&doc); err == nil {
			tagDistribution = append(tagDistribution, map[string]interface{}{
				"tag":   doc["_id"],
				"count": doc["count"],
			})
		}
	}
	return tagDistribution, nil
}

func getDomainDistribution(ctx context.Context, userID string) ([]map[string]interface{}, error) {
	domainDistribution := []map[string]interface{}{}
	domainPipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.D{
			{Key: "user_id", Value: userID},
			{Key: "is_active", Value: true},
		}}},
		bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$domain"},
			{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}}}},
	}
	domainCursor, err := DB.Collection.Aggregate(ctx, domainPipeline)
	if err != nil {
		return domainDistribution, nil
	}
	defer domainCursor.Close(ctx)
	for domainCursor.Next(ctx) {
		var doc map[string]interface{}
		if err := domainCursor.Decode(&doc); err == nil {
			domainDistribution = append(domainDistribution, map[string]interface{}{
				"domain": doc["_id"],
				"count":  doc["count"],
			})
		}
	}
	return domainDistribution, nil
}

func getTopLinks(ctx context.Context, userID string) ([]map[string]interface{}, error) {
	topLinks := []map[string]interface{}{}
	topPipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.D{
			{Key: "user_id", Value: userID},
			{Key: "is_active", Value: true},
			{Key: "clicks", Value: bson.D{{Key: "$gt", Value: 0}}},
		}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "clicks", Value: -1}}}},
		bson.D{{Key: "$limit", Value: 10}},
		bson.D{{Key: "$project", Value: bson.D{
			{Key: "short_url", Value: 1},
			{Key: "long_url", Value: 1},
			{Key: "domain", Value: 1},
			{Key: "tags", Value: 1},
			{Key: "clicks", Value: 1},
			{Key: "created_at", Value: 1},
			{Key: "expires_at", Value: 1},
			{Key: "is_active", Value: 1},
			{Key: "_id", Value: 0},
		}}},
	}
	topCursor, err := DB.Collection.Aggregate(ctx, topPipeline)
	if err != nil {
		return topLinks, nil
	}
	defer topCursor.Close(ctx)
	for topCursor.Next(ctx) {
		var doc map[string]interface{}
		if err := topCursor.Decode(&doc); err == nil {
			topLinks = append(topLinks, doc)
		}
	}
	return topLinks, nil
}

// StartCleanupWorker starts a background goroutine for periodic cleanup of expired URLs
func StartCleanupWorker() {
	go func() {
		log.Println("🧹 Starting cleanup worker for expired URLs...")
		ticker := time.NewTicker(1 * time.Hour) // Run cleanup every hour
		defer ticker.Stop()
		for range ticker.C {
			if err := CleanupExpiredURLs(); err != nil {
				log.Printf("Error during cleanup: %v", err)
			} else {
				log.Println("✅ Cleanup worker completed successfully")
			}
		}
	}()
}

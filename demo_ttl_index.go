// MongoDB TTL index for demo_urls collection
// Run this once in your DB setup or migration script
package main

import (
	"context"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func EnsureDemoURLTTLIndex() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	collection := DB.Database.Collection("demo_urls")
	// TTL index on expires_at field (auto-delete after expiry)
	indexModel := mongo.IndexModel{
		Keys:    bson.D{{Key: "expires_at", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(0),
	}
	_, err := collection.Indexes().CreateOne(ctx, indexModel)
	if err != nil {
		log.Printf("Failed to create TTL index for demo_urls: %v", err)
		return err
	}
	log.Println("✅ TTL index for demo_urls.expires_at ensured!")
	return nil
}

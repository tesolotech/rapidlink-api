package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type DemoURL struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	ShortURL  string             `bson:"short_url" json:"short_url"`
	LongURL   string             `bson:"long_url" json:"long_url"`
	Domain    string             `bson:"domain" json:"domain"`
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
	ExpiresAt time.Time          `bson:"expires_at" json:"expires_at"`
	SessionID string             `bson:"session_id" json:"session_id"`
}

// Handler for anonymous/demo shortener
func rapidLinkDemo(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sessionCookie, err := r.Cookie("rapidlink_demo_session")
	if err != nil || sessionCookie.Value == "" {
		// Generate a new session ID
		sessionID := primitive.NewObjectID().Hex()
		http.SetCookie(w, &http.Cookie{
			Name:     "rapidlink_demo_session",
			Value:    sessionID,
			Path:     "/",
			Expires:  time.Now().Add(1 * time.Hour),
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
		sessionCookie = &http.Cookie{Name: "rapidlink_demo_session", Value: sessionID}
	}

	// Count how many demo URLs this session has created
	collection := DB.Database.Collection("demo_urls")
	count, err := collection.CountDocuments(ctx, bson.M{"session_id": sessionCookie.Value})
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if count >= 5 {
		http.Error(w, "Demo limit reached. Please sign up to create more short URLs.", http.StatusForbidden)
		return
	}

	var req struct {
		LongURL string `json:"long_url"`
		Domain  string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	// Generate short code (reuse your existing logic)
	code := generateReadableCode(req.LongURL)

	// Set expiry to session expiry (1h for demo)
	expiresAt := time.Now().Add(1 * time.Hour)

	demoURL := DemoURL{
		ShortURL:  code,
		LongURL:   req.LongURL,
		Domain:    req.Domain,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: expiresAt,
		SessionID: sessionCookie.Value,
	}
	_, err = collection.InsertOne(ctx, demoURL)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(demoURL)
}

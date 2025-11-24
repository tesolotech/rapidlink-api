package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// GET /rapidlink-demo - fetch all demo URLs for the current session
func getDemoURLs(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sessionCookie, err := r.Cookie("rapidlink_demo_session")
	if err != nil || sessionCookie.Value == "" {
		http.Error(w, "No demo session found", http.StatusUnauthorized)
		return
	}

	collection := DB.Database.Collection("demo_urls")
	cursor, err := collection.Find(ctx, map[string]interface{}{"session_id": sessionCookie.Value})
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer cursor.Close(ctx)

	var urls []DemoURL
	for cursor.Next(ctx) {
		var url DemoURL
		if err := cursor.Decode(&url); err == nil {
			urls = append(urls, url)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(urls)
}

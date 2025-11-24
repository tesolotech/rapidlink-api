package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/crypto/bcrypt"
)

// JWT configuration
var (
	JWTSecret     []byte
	TokenDuration = 1 * time.Hour // Token expires in 1 hour
)

// User represents a user in the system
type User struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	Username  string             `bson:"username" json:"username"`
	Email     string             `bson:"email" json:"email"`
	Password  string             `bson:"password" json:"-"` // Never return password in JSON
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
	IsActive  bool               `bson:"is_active" json:"is_active"`
    RefreshToken string             `bson:"refresh_token,omitempty" json:"-"` // Store hashed refresh token
    RefreshTokenExpiry time.Time     `bson:"refresh_token_expiry,omitempty" json:"-"`
}
// GenerateRefreshToken creates a new secure random refresh token
func GenerateRefreshToken() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// HashRefreshToken hashes the refresh token for storage
func HashRefreshToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// SetRefreshToken sets a new refresh token and expiry for a user in the DB
func SetRefreshToken(userID string, refreshToken string, expiry time.Time) error {
	if DB == nil {
		return fmt.Errorf("database not connected")
	}
	objectID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return fmt.Errorf("invalid user ID")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	hashed := HashRefreshToken(refreshToken)
	update := bson.M{
		"$set": bson.M{
			"refresh_token": hashed,
			"refresh_token_expiry": expiry,
		},
	}
	_, err = DB.Database.Collection("users").UpdateOne(ctx, bson.M{"_id": objectID}, update)
	return err
}

// ValidateRefreshToken checks if the refresh token is valid for the user
func ValidateRefreshToken(user *User, refreshToken string) bool {
	if user == nil || user.RefreshToken == "" {
		return false
	}
	if time.Now().After(user.RefreshTokenExpiry) {
		return false
	}
	return user.RefreshToken == HashRefreshToken(refreshToken)
}

// ClearRefreshToken removes the refresh token from the user (on logout or rotation)
func ClearRefreshToken(userID string) error {
	if DB == nil {
		return fmt.Errorf("database not connected")
	}
	objectID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return fmt.Errorf("invalid user ID")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	update := bson.M{
		"$unset": bson.M{
			"refresh_token":        "",
			"refresh_token_expiry": "",
		},
	}
	_, err = DB.Database.Collection("users").UpdateOne(ctx, bson.M{"_id": objectID}, update)
	return err
}

// Claims represents JWT claims
type Claims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	jwt.RegisteredClaims
}

// AuthRequest represents login/register request
type AuthRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// AuthResponse represents authentication response
type AuthResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      User      `json:"user"`
}

// InitJWT initializes the JWT secret
func InitJWT() {
	// Try to get secret from environment variable
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		// Generate a random secret if not provided
		randomBytes := make([]byte, 32)
		_, err := rand.Read(randomBytes)
		if err != nil {
			log.Fatal("Failed to generate JWT secret:", err)
		}
		secret = hex.EncodeToString(randomBytes)
		log.Println("Generated JWT secret. In production, set JWT_SECRET environment variable.")
	}
	JWTSecret = []byte(secret)
}

// GenerateToken creates a new JWT token for a user
func GenerateToken(user *User) (string, time.Time, error) {
	expiresAt := time.Now().Add(TokenDuration)

	claims := &Claims{
		UserID:   user.ID.Hex(),
		Username: user.Username,
		Email:    user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "url-shortener",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(JWTSecret)

	return tokenString, expiresAt, err
}

// ValidateToken validates a JWT token and returns the claims
func ValidateToken(tokenString string) (*Claims, error) {
	claims := &Claims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return JWTSecret, nil
	})

	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	return claims, nil
}

func JWTMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		// Check if it's a Bearer token
		bearerToken := strings.Split(authHeader, " ")
		if len(bearerToken) != 2 || bearerToken[0] != "Bearer" {
			http.Error(w, "Invalid authorization header format. Use: Bearer <token>", http.StatusUnauthorized)
			return
		}

		tokenString := bearerToken[1]
		claims, err := ValidateToken(tokenString)
		if err != nil {
			http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		// Add user info to request context
		ctx := context.WithValue(r.Context(), "user_id", claims.UserID)
		ctx = context.WithValue(ctx, "username", claims.Username)
		ctx = context.WithValue(ctx, "email", claims.Email)

		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// HashPassword hashes a password using bcrypt
func HashPassword(password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashedPassword), nil
}

// CheckPassword compares a password with its hash
func CheckPassword(password, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// CreateUser creates a new user in the database (legacy)
func CreateUser(username, email, password string) (*User, error) {
	return CreateUserWithTransaction(username, email, password)
}

// CreateUserWithTransaction creates a new user using MongoDB transactions for consistency
func CreateUserWithTransaction(username, email, password string) (*User, error) {
	if DB == nil {
		return nil, fmt.Errorf("database not connected")
	}

	session, err := DB.Client.StartSession()
	if err != nil {
		return nil, fmt.Errorf("failed to start session: %v", err)
	}
	defer session.EndSession(context.Background())

	var user *User
	err = mongo.WithSession(context.Background(), session, func(sc mongo.SessionContext) error {
		// Hash the password
		hashedPassword, err := HashPassword(password)
		if err != nil {
			return fmt.Errorf("failed to hash password: %v", err)
		}

		// Check if user already exists (within transaction)
		var existingUser User
		userCollection := DB.Database.Collection("users")
		err = userCollection.FindOne(sc, bson.D{
			{"$or", bson.A{
				bson.D{{"username", username}},
				bson.D{{"email", email}},
			}},
			{"is_active", true},
		}).Decode(&existingUser)

		if err == nil {
			return fmt.Errorf("user with this username or email already exists")
		} else if err != mongo.ErrNoDocuments {
			return fmt.Errorf("error checking existing user: %v", err)
		}

		// Create new user
		user = &User{
			Username:  username,
			Email:     email,
			Password:  hashedPassword,
			CreatedAt: time.Now().UTC(),
			IsActive:  true,
		}

		// Insert the new user
		result, err := userCollection.InsertOne(sc, user)
		if err != nil {
			return fmt.Errorf("failed to create user: %v", err)
		}

		user.ID = result.InsertedID.(primitive.ObjectID)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return user, nil
}

// GetUserByCredentials retrieves a user by username/email and verifies password (optimized)
func GetUserByCredentials(usernameOrEmail, password string) (*User, error) {
	if DB == nil {
		return nil, fmt.Errorf("database not connected")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second) // Reduced timeout for faster response
	defer cancel()

	var user User
	// Use optimized query that leverages compound indexes
	err := DB.Database.Collection("users").FindOne(ctx, bson.D{
		{Key: "$or", Value: bson.A{
			bson.D{{Key: "username", Value: usernameOrEmail}, {Key: "is_active", Value: true}},
			bson.D{{Key: "email", Value: usernameOrEmail}, {Key: "is_active", Value: true}},
		}},
	}).Decode(&user)

	if err == mongo.ErrNoDocuments {
		return nil, fmt.Errorf("user not found or inactive")
	}
	if err != nil {
		return nil, fmt.Errorf("database error: %v", err)
	}

	// Check password
	if err := CheckPassword(password, user.Password); err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	return &user, nil
}

// GetUserByID retrieves a user by ID
func GetUserByID(userID string) (*User, error) {
	if DB == nil {
		return nil, fmt.Errorf("database not connected")
	}

	objectID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user User
	err = DB.Database.Collection("users").FindOne(ctx, bson.D{
		{"_id", objectID},
		{"is_active", true},
	}).Decode(&user)

	if err == mongo.ErrNoDocuments {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("database error: %v", err)
	}

	return &user, nil
}

// GetUserProfile returns user profile with statistics
func GetUserProfile(userID string) (map[string]interface{}, error) {
	user, err := GetUserByID(userID)
	if err != nil {
		return nil, err
	}

	// Get user statistics using optimized aggregation
	stats, err := GetUserStatsOptimized(userID)
	if err != nil {
		log.Printf("Warning: Could not get user stats: %v", err)
		stats = map[string]interface{}{
			"total_urls":         0,
			"total_clicks":       0,
			"avg_clicks_per_url": 0,
		}
	}

	profile := map[string]interface{}{
		"user": map[string]interface{}{
			"id":         user.ID.Hex(),
			"username":   user.Username,
			"email":      user.Email,
			"created_at": user.CreatedAt,
			"is_active":  user.IsActive,
		},
		"statistics": stats,
	}

	return profile, nil
}

package main

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"math/rand"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// ============================================================================
// BASE58 ENCODING CONFIGURATION
// ============================================================================

// Base58 alphabet (Bitcoin-style) - excludes confusing characters 0, O, I, l
const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// encodeBase58 converts a big integer to base58 string
func encodeBase58(num *big.Int) string {
	if num.Cmp(big.NewInt(0)) == 0 {
		return "1"
	}

	var result []byte
	base := big.NewInt(58)
	zero := big.NewInt(0)
	mod := new(big.Int)
	numCopy := new(big.Int).Set(num) // Create a copy to avoid modifying original

	for numCopy.Cmp(zero) > 0 {
		numCopy.DivMod(numCopy, base, mod)
		result = append([]byte{base58Alphabet[mod.Int64()]}, result...)
	}

	return string(result)
}

// padBase58 ensures minimum length by prepending '1' characters
func padBase58(code string, minLength int) string {
	for len(code) < minLength {
		code = "1" + code // "1" represents zero in base58
	}
	return code
}

// generateBase58Suffix creates a random base58 suffix
func generateBase58Suffix(length int) string {
	suffix := ""
	for i := 0; i < length; i++ {
		suffix += string(base58Alphabet[rand.Intn(58)])
	}
	return suffix
}

// sanitizeStringSlice sanitizes each string in a slice
func sanitizeStringSlice(input []string) []string {
	result := make([]string, len(input))
	for i, s := range input {
		result[i] = sanitizeInput(s)
	}
	return result
}

// ============================================================================
// DATA STRUCTURES
// ============================================================================

type ClickHistory struct {
	Timestamp time.Time `bson:"timestamp" json:"timestamp"`
	IP        string    `bson:"ip" json:"ip"`
	UserAgent string    `bson:"user_agent" json:"user_agent"`
}

// ShortenRequest represents the JSON payload for URL shortening
type ShortenRequest struct {
	LongURL string   `json:"long-url"`
	Custom  string   `json:"custom,omitempty"`
	Expires string   `json:"expires,omitempty"`
	Domain  string   `json:"domain,omitempty"`
	Tags    []string `json:"tags,omitempty"`
}

type URLData struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	ShortURL     string             `bson:"short_url" json:"short-url"`
	LongURL      string             `bson:"long_url" json:"long-url"`
	Domain       string             `bson:"domain,omitempty" json:"domain,omitempty"`
	Tags         []string           `bson:"tags,omitempty" json:"tags,omitempty"`
	UserID       string             `bson:"user_id" json:"user_id"`
	CreatedAt    time.Time          `bson:"created_at" json:"created-at"`
	ExpiresAt    *time.Time         `bson:"expires_at,omitempty" json:"expires-at,omitempty"`
	Clicks       int                `bson:"clicks" json:"clicks"`
	IsActive     bool               `bson:"is_active" json:"is-active"`
	LastClicked  *time.Time         `bson:"last_clicked,omitempty" json:"last-clicked,omitempty"`
	ClickHistory []ClickHistory     `bson:"click_history" json:"click_history"`
}

// ============================================================================
// BULK UPLOAD DATA STRUCTURES
// ============================================================================

type BulkURLRequest struct {
	LongURL     string   `json:"long_url"`
	Domain      string   `json:"domain,omitempty"`
	CustomAlias string   `json:"custom,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Expires     string   `json:"expires,omitempty"`
}

type BulkURLResult struct {
	LongURL   string   `json:"long_url"`
	ShortURL  string   `json:"short_url,omitempty"`
	Domain    string   `json:"domain,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Success   bool     `json:"success"`
	Error     string   `json:"error,omitempty"`
	CreatedAt string   `json:"created_at,omitempty"`
}

type BulkResponse struct {
	TotalProcessed int             `json:"total_processed"`
	Successful     int             `json:"successful"`
	Failed         int             `json:"failed"`
	Results        []BulkURLResult `json:"results"`
	ProcessingTime string          `json:"processing_time"`
}

// ============================================================================
// AUTHENTICATION HANDLERS
// ============================================================================

// register handles POST /auth/register requests
func register(w http.ResponseWriter, r *http.Request) {
	clientIP := getClientIP(r)

	var req AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("error decoding register request: %v", err)
		logSecurityEvent("INVALID_REGISTER_PAYLOAD", "", clientIP, r.UserAgent(),
			"Invalid JSON payload", "WARN")
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}

	// Sanitize all inputs to prevent XSS
	req.Username = sanitizeInput(req.Username)
	req.Email = sanitizeInput(req.Email)
	req.Password = sanitizeInput(req.Password)

	// Validate inputs with enhanced security checks
	if !validateUsername(req.Username) {
		logSecurityEvent("INVALID_USERNAME", "", clientIP, r.UserAgent(),
			"Invalid username format: "+req.Username, "WARN")
		http.Error(w, "Invalid username format. Use 3-30 alphanumeric characters, dots, underscores, or hyphens", http.StatusBadRequest)
		return
	}

	if !validateEmail(req.Email) {
		logSecurityEvent("INVALID_EMAIL", "", clientIP, r.UserAgent(),
			"Invalid email format: "+req.Email, "WARN")
		http.Error(w, "Invalid email format", http.StatusBadRequest)
		return
	}

	if !validatePassword(req.Password) {
		logSecurityEvent("WEAK_PASSWORD", "", clientIP, r.UserAgent(),
			"Password does not meet security requirements", "WARN")
		http.Error(w, "Password must be 8-128 characters with at least one letter and one number", http.StatusBadRequest)
		return
	}

	// Create user with enhanced security
	user, err := CreateUserWithTransaction(req.Username, req.Email, req.Password)
	if err != nil {
		log.Printf("error creating user: %v", err)
		logSecurityEvent("USER_CREATION_FAILED", "", clientIP, r.UserAgent(),
			err.Error(), "ERROR")
		if strings.Contains(err.Error(), "already exists") {
			http.Error(w, "user with this username or email already exists", http.StatusConflict)
		} else {
			http.Error(w, "failed to create user", http.StatusInternalServerError)
		}
		return
	}

	// Generate access token
	token, expiresAt, err := GenerateToken(user)
	if err != nil {
		log.Printf("error generating token: %v", err)
		logSecurityEvent("TOKEN_GENERATION_FAILED", user.ID.Hex(), clientIP, r.UserAgent(),
			"Token generation failed", "ERROR")
		http.Error(w, "failed to generate token", http.StatusInternalServerError)
		return
	}

	// Generate refresh token
	refreshToken, err := GenerateRefreshToken()
	if err != nil {
		log.Printf("error generating refresh token: %v", err)
		http.Error(w, "failed to generate refresh token", http.StatusInternalServerError)
		return
	}
	refreshExpiry := time.Now().Add(7 * 24 * time.Hour) // 7 days
	if err := SetRefreshToken(user.ID.Hex(), refreshToken, refreshExpiry); err != nil {
		log.Printf("error saving refresh token: %v", err)
		http.Error(w, "failed to save refresh token", http.StatusInternalServerError)
		return
	}

	// Set refresh token as HttpOnly, Secure cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		Path:     "/",
		Expires:  refreshExpiry,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})

	// Log successful registration
	logSecurityEvent("USER_REGISTERED", user.ID.Hex(), clientIP, r.UserAgent(),
		"User successfully registered", "INFO")

	response := AuthResponse{
		Token:     token,
		ExpiresAt: expiresAt,
		User:      *user,
	}

	w.Header().Set("Content-Type", "application/json")
	addSecurityHeaders(w)
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("error encoding register response: %v", err)
	}
}

// login handles POST /auth/login requests
func login(w http.ResponseWriter, r *http.Request) {
	clientIP := getClientIP(r)

	var req struct {
		UsernameOrEmail string `json:"username_or_email"`
		Password        string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("error decoding login request: %v", err)
		logSecurityEvent("INVALID_LOGIN_PAYLOAD", "", clientIP, r.UserAgent(),
			"Invalid JSON payload", "WARN")
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}

	// Sanitize inputs to prevent XSS
	req.UsernameOrEmail = sanitizeInput(req.UsernameOrEmail)
	req.Password = sanitizeInput(req.Password)

	// Validate required fields
	if req.UsernameOrEmail == "" || req.Password == "" {
		logSecurityEvent("INCOMPLETE_LOGIN_DATA", "", clientIP, r.UserAgent(),
			"Missing username/email or password", "WARN")
		http.Error(w, "username/email and password are required", http.StatusBadRequest)
		return
	}

	// Validate email format if it looks like an email
	if strings.Contains(req.UsernameOrEmail, "@") && !validateEmail(req.UsernameOrEmail) {
		logSecurityEvent("INVALID_LOGIN_EMAIL", "", clientIP, r.UserAgent(),
			"Invalid email format in login", "WARN")
		http.Error(w, "Invalid email format", http.StatusBadRequest)
		return
	}

	// Get user and verify password
	user, err := GetUserByCredentials(req.UsernameOrEmail, req.Password)
	if err != nil {
		log.Printf("login failed for %s: %v", req.UsernameOrEmail, err)
		logSecurityEvent("LOGIN_FAILED", "", clientIP, r.UserAgent(),
			"Login failed for: "+req.UsernameOrEmail, "WARN")
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	// Generate access token
	token, expiresAt, err := GenerateToken(user)
	if err != nil {
		log.Printf("error generating token: %v", err)
		logSecurityEvent("TOKEN_GENERATION_FAILED", user.ID.Hex(), clientIP, r.UserAgent(),
			"Token generation failed after successful login", "ERROR")
		http.Error(w, "failed to generate token", http.StatusInternalServerError)
		return
	}

	// Generate refresh token
	refreshToken, err := GenerateRefreshToken()
	if err != nil {
		log.Printf("error generating refresh token: %v", err)
		http.Error(w, "failed to generate refresh token", http.StatusInternalServerError)
		return
	}
	refreshExpiry := time.Now().Add(7 * 24 * time.Hour) // 7 days
	if err := SetRefreshToken(user.ID.Hex(), refreshToken, refreshExpiry); err != nil {
		log.Printf("error saving refresh token: %v", err)
		http.Error(w, "failed to save refresh token", http.StatusInternalServerError)
		return
	}

	// Set refresh token as HttpOnly, Secure cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		Path:     "/",
		Expires:  refreshExpiry,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})

	// Log successful login
	logSecurityEvent("USER_LOGIN", user.ID.Hex(), clientIP, r.UserAgent(),
		"User successfully logged in", "INFO")

	response := AuthResponse{
		Token:     token,
		ExpiresAt: expiresAt,
		User:      *user,
	}

	w.Header().Set("Content-Type", "application/json")
	addSecurityHeaders(w)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("error encoding login response: %v", err)
	}
}

// profile handles GET /auth/profile requests (protected) - Enhanced with statistics
func profile(w http.ResponseWriter, r *http.Request) {
	// Get user ID from context (set by JWT middleware)
	userID, ok := r.Context().Value("user_id").(string)
	if !ok {
		http.Error(w, "user information not found", http.StatusInternalServerError)
		return
	}

	// Get user profile with statistics using optimized function
	profile, err := GetUserProfile(userID)
	if err != nil {
		log.Printf("error getting user profile: %v", err)
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "user not found", http.StatusNotFound)
		} else {
			http.Error(w, "failed to get user profile", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	addSecurityHeaders(w)
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Profile retrieved successfully",
		"data":    profile,
	}); err != nil {
		log.Printf("error encoding profile response: %v", err)
	}
}

// validateToken handles POST /auth/validate requests
func validateToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("error decoding validate request: %v", err)
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}

	if req.Token == "" {
		http.Error(w, "token is required", http.StatusBadRequest)
		return
	}

	claims, err := ValidateToken(req.Token)
	if err != nil {
		log.Printf("token validation failed: %v", err)
		http.Error(w, "invalid or expired token", http.StatusUnauthorized)
		return
	}

	response := map[string]interface{}{
		"valid":    true,
		"user_id":  claims.UserID,
		"username": claims.Username,
		"email":    claims.Email,
		"expires":  claims.ExpiresAt.Time,
	}

	w.Header().Set("Content-Type", "application/json")
	addSecurityHeaders(w)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("error encoding validate response: %v", err)
	}
}

// refreshTokenHandler handles POST /auth/refresh requests (secure, cookie-based)
func refreshTokenHandler(w http.ResponseWriter, r *http.Request) {
	// Get refresh token from HttpOnly cookie
	cookie, err := r.Cookie("refresh_token")
	if err != nil || cookie.Value == "" {
		http.Error(w, "Refresh token missing", http.StatusUnauthorized)
		return
	}
	refreshToken := cookie.Value

	// Find user by refresh token (must scan for matching hash)
	if DB == nil {
		http.Error(w, "Database not connected", http.StatusInternalServerError)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	hashed := HashRefreshToken(refreshToken)
	var user User
	err = DB.Database.Collection("users").FindOne(ctx, bson.M{"refresh_token": hashed}).Decode(&user)
	if err != nil {
		http.Error(w, "Invalid refresh token", http.StatusUnauthorized)
		return
	}
	// Validate expiry
	if !ValidateRefreshToken(&user, refreshToken) {
		// Clear cookie and DB
		_ = ClearRefreshToken(user.ID.Hex())
		http.SetCookie(w, &http.Cookie{
			Name:     "refresh_token",
			Value:    "",
			Path:     "/",
			Expires:  time.Now().Add(-1 * time.Hour),
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
		})
		http.Error(w, "Refresh token expired or invalid", http.StatusUnauthorized)
		return
	}

	// Rotate: generate new refresh token
	newRefreshToken, err := GenerateRefreshToken()
	if err != nil {
		http.Error(w, "Failed to generate refresh token", http.StatusInternalServerError)
		return
	}
	refreshExpiry := time.Now().Add(7 * 24 * time.Hour)
	if err := SetRefreshToken(user.ID.Hex(), newRefreshToken, refreshExpiry); err != nil {
		http.Error(w, "Failed to save refresh token", http.StatusInternalServerError)
		return
	}
	// Set new refresh token cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    newRefreshToken,
		Path:     "/",
		Expires:  refreshExpiry,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})

	// Issue new access token
	accessToken, expiresAt, err := GenerateToken(&user)
	if err != nil {
		http.Error(w, "Failed to generate access token", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"success":    true,
		"message":    "Token refreshed successfully",
		"token":      accessToken,
		"expires_at": expiresAt,
	}

	w.Header().Set("Content-Type", "application/json")
	addSecurityHeaders(w)
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding refresh token response: %v", err)
	}
}

// ============================================================================
// URL MANAGEMENT HANDLERS
// ============================================================================

// shorten handles PUT /url requests with payload:
//
//	{
//	  "long-url": "https://example.com/very/long/url",     // required: original URL to shorten
//	  "expires": "2025-12-31T23:59:59Z",                   // optional: define expiration date and time of short URL; default to 5 years
//	  "custom": "my-custom-url"                            // optional: define custom short URL
//	}
//
// Response:
//
//	{
//	  "long-url": "https://example.com/very/long/url",
//	  "short-url": "abc123",
//	  "created-at": "2025-11-17T10:30:00Z",
//	  "expires-at": "2030-11-17T10:30:00Z",
//	  "is-active": true
//	}
func shorten(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(string)
	clientIP := getClientIP(r)
	var req ShortenRequest
	log.Printf("req: %+v", req)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("error decoding shorten request: %v", err)
		logSecurityEvent("INVALID_SHORTEN_PAYLOAD", userID, clientIP, r.UserAgent(),
			"Invalid JSON payload", "WARN")
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}

	// Sanitize inputs to prevent XSS and other attacks
	req.LongURL = sanitizeInput(req.LongURL)
	req.Custom = sanitizeInput(req.Custom)
	req.Expires = sanitizeInput(req.Expires)
	req.Domain = sanitizeInput(req.Domain)
	req.Tags = sanitizeStringSlice(req.Tags)
	// Default domain to BASE_URL if not provided
	if req.Domain == "" {
		req.Domain = os.Getenv("BASE_URL")
	}

	// Validate URL with enhanced security checks
	if !validateURL(req.LongURL) {
		logSecurityEvent("INVALID_URL_FORMAT", userID, clientIP, r.UserAgent(),
			"Invalid URL format: "+req.LongURL, "WARN")
		http.Error(w, "Invalid URL format. Must be a valid HTTP or HTTPS URL (no localhost/internal IPs)", http.StatusBadRequest)
		return
	}

	// Validate domain if provided
	if req.Domain != "" && !validateURL(req.Domain) {
		logSecurityEvent("INVALID_DOMAIN_FORMAT", userID, clientIP, r.UserAgent(),
			"Invalid domain format: "+req.Domain, "WARN")
		http.Error(w, "Invalid domain format. Must be a valid HTTP or HTTPS URL (no localhost/internal IPs)", http.StatusBadRequest)
		return
	}

	// Validate custom short URL if provided
	if req.Custom != "" && !validateCustomURL(req.Custom) {
		logSecurityEvent("INVALID_CUSTOM_URL", userID, clientIP, r.UserAgent(),
			"Invalid custom URL format: "+req.Custom, "WARN")
		http.Error(w, "Custom URL must be 3-20 characters, alphanumeric with hyphens/underscores only", http.StatusBadRequest)
		return
	}

	// Check if this URL already exists for this user (1-to-1 mapping)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var existingURL URLData
	err := DB.Collection.FindOne(ctx, bson.D{
		{Key: "long_url", Value: req.LongURL},
		{Key: "domain", Value: req.Domain},
		{Key: "user_id", Value: userID},
		{Key: "is_active", Value: true},
	}).Decode(&existingURL)

	if err == nil {
		// URL already exists for this user, return existing short URL
		// Format with BASE_URL for consistent client response
		// existingURL.ShortURL = os.Getenv("BASE_URL") + "/" + existingURL.ShortURL
		log.Printf("Returning existing short URL for user %s: %s", userID, existingURL.ShortURL)
		w.Header().Set("Content-Type", "application/json")
		addSecurityHeaders(w)
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(existingURL); err != nil {
			log.Printf("error encoding existing URL response: %v", err)
		}
		return
	} else if err != mongo.ErrNoDocuments {
		log.Printf("error checking existing URL: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	// Use custom ID if provided, otherwise generate a Base58 short code
	code := req.Custom
	if code == "" {
		// Generate Base58 encoded short code
		code = generateReadableCode(req.LongURL)
	}

	// Parse expiry time if provided, otherwise default to 5 years
	var expiresAt *time.Time
	if req.Expires != "" {
		if expiry, err := time.Parse(time.RFC3339, req.Expires); err == nil {
			expiresAt = &expiry
		} else {
			http.Error(w, "invalid expires format, use RFC3339 (e.g., 2025-12-31T23:59:59Z)", http.StatusBadRequest)
			return
		}
	} else {
		// Default to 5 years from now
		defaultExpiry := time.Now().UTC().AddDate(5, 0, 0)
		expiresAt = &defaultExpiry
	}

	// Create URL data
	urlData := &URLData{
		ShortURL:     code,
		LongURL:      req.LongURL,
		Domain:       req.Domain,
		Tags:         req.Tags,
		UserID:       userID,
		CreatedAt:    time.Now().UTC(),
		ExpiresAt:    expiresAt,
		Clicks:       0,
		IsActive:     true,
		ClickHistory: []ClickHistory{},
	}

	// Check if short URL already exists (collision detection)
	var existingShort URLData
	err = DB.Collection.FindOne(ctx, bson.D{{Key: "short_url", Value: code}}).Decode(&existingShort)
	if err == nil {
		// Collision detected, generate a new code with suffix
		log.Printf("Short URL collision detected: %s", code)
		code = code + generateBase58Suffix(2)
		urlData.ShortURL = code
	} else if err != mongo.ErrNoDocuments {
		log.Printf("error checking short URL collision: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	// Insert into MongoDB
	result, err := DB.Collection.InsertOne(ctx, urlData)
	if err != nil {
		log.Printf("error inserting URL data: %v", err)
		http.Error(w, "failed to create short URL", http.StatusInternalServerError)
		return
	}
	urlData.ID = result.InsertedID.(primitive.ObjectID)

	// Format short URL with BASE_URL for client response
	// urlData.ShortURL = os.Getenv("BASE_URL") + "/" + code

	// Log successful URL creation
	logSecurityEvent("URL_CREATED", userID, clientIP, r.UserAgent(),
		"URL created: "+req.LongURL+" -> "+code, "INFO")

	log.Printf("✅ Base58 URL created: %s → %s for user %s", req.LongURL, code, userID)

	w.Header().Set("Content-Type", "application/json")
	addSecurityHeaders(w)
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(urlData); err != nil {
		log.Printf("error encoding shorten response: %v", err)
	}
}

// generateReadableCode creates deterministic, collision-resistant short codes using Base58 encoding
func generateReadableCode(longURL string) string {
	// Create SHA256 hash for deterministic generation (maintains 1:1 mapping)
	hash := sha256.Sum256([]byte(longURL))

	// Convert first 8 bytes to big integer for base58 conversion
	hashInt := new(big.Int).SetBytes(hash[:8])

	// Convert to base58 - produces shorter, more readable URLs
	base58Code := encodeBase58(hashInt)

	// Ensure minimum length of 6 characters for consistency
	if len(base58Code) < 6 {
		base58Code = padBase58(base58Code, 6)
	}

	// Truncate if too long (rare case)
	if len(base58Code) > 10 {
		base58Code = base58Code[:10]
	}

	// Check for collision in database (rare with SHA256 + base58)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Safety check for database connection
	if DB == nil || DB.Collection == nil {
		log.Printf("Database not connected, using base58 fallback")
		return generateBase58Suffix(7) // Fallback to random base58
	}

	// Check if code exists (very rare collision)
	var existing URLData
	err := DB.Collection.FindOne(ctx, bson.D{{Key: "short_url", Value: base58Code}}).Decode(&existing)
	if err == mongo.ErrNoDocuments {
		// Code is unique - perfect!
		return base58Code
	}
	if err != nil {
		log.Printf("Error checking base58 code uniqueness: %v", err)
		// Add random suffix as fallback
		return base58Code + generateBase58Suffix(2)
	}

	// Rare collision detected - add random suffix
	log.Printf("Base58 collision detected for URL")
	return base58Code + generateBase58Suffix(2)
}

// RandString generates a random string using Base58 characters for consistency
func RandString(n int) string {
	// Use base58 alphabet for all random generation
	b := make([]rune, n)
	for i := range b {
		b[i] = rune(base58Alphabet[rand.Intn(len(base58Alphabet))])
	}
	return string(b)
}

// analytics returns user's URL statistics with optimized queries
func analytics(w http.ResponseWriter, r *http.Request) {
	// Get user ID from context (set by JWT middleware)
	userID, ok := r.Context().Value("user_id").(string)
	if !ok {
		http.Error(w, "user information not found", http.StatusInternalServerError)
		return
	}

	// Parse pagination parameters
	pageStr := r.URL.Query().Get("page")
	pageSizeStr := r.URL.Query().Get("pageSize")
	limitStr := r.URL.Query().Get("limit") // fallback for legacy
	page := 1
	pageSize := 20
	if pageStr != "" {
		if parsedPage, err := strconv.Atoi(pageStr); err == nil && parsedPage > 0 {
			page = parsedPage
		}
	}
	if pageSizeStr != "" {
		if parsedSize, err := strconv.Atoi(pageSizeStr); err == nil && parsedSize > 0 && parsedSize <= 100 {
			pageSize = parsedSize
		}
	} else if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 100 {
			pageSize = parsedLimit
		}
	}
	skip := (page - 1) * pageSize

	// Get user statistics using optimized aggregation
	stats, err := GetUserStatsOptimized(userID)
	if err != nil {
		log.Printf("Stats error for user %s: %v", userID, err)
		stats = map[string]interface{}{
			"total_urls":         0,
			"total_clicks":       0,
			"avg_clicks_per_url": 0,
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get total count for pagination
	totalCount, err := DB.Collection.CountDocuments(ctx, bson.M{"user_id": userID, "is_active": true})
	if err != nil {
		log.Printf("Count error for user %s: %v", userID, err)
		totalCount = 0
	}

	// Get user URLs with pagination
	urls, err := GetUserURLsPaginated(userID, skip, pageSize)
	if err != nil {
		log.Printf("Analytics error for user %s: %v", userID, err)
		http.Error(w, "Failed to retrieve analytics", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	addSecurityHeaders(w)
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"message":    "Analytics retrieved successfully",
		"statistics": stats,
		"urls":       urls,
		"page":       page,
		"pageSize":   pageSize,
		"total":      totalCount,
		"count":      len(urls),
	}); err != nil {
		log.Printf("error encoding analytics response: %v", err)
	}
}

// ============================================================================
// URL REDIRECT HANDLER
// ============================================================================

// redirect handles GET /{short-url} requests
func redirect(w http.ResponseWriter, r *http.Request) {
	// Extract the short URL from the request path
	shortURL := strings.TrimPrefix(r.URL.Path, "/")

	// Sanitize short URL input to prevent injection attacks
	shortURL = sanitizeInput(shortURL)

	// Validate short URL format and length
	if shortURL == "" || shortURL == "url" || shortURL == "analytics" ||
		len(shortURL) > 50 || !validateCustomURL(shortURL) {
		logSecurityEvent("INVALID_SHORT_URL_ACCESS", "", getClientIP(r), r.UserAgent(),
			"Invalid short URL attempted: "+shortURL, "WARN")
		http.NotFound(w, r)
		return
	}

	// Safety check for database connection
	if DB == nil || DB.Collection == nil {
		log.Printf("Database not connected")
		http.Error(w, "database connection error", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Try to find in main URLs collection (authenticated/registered users)
	var urlData URLData
	err := DB.Collection.FindOne(ctx, bson.D{
		{Key: "short_url", Value: shortURL},
		{Key: "is_active", Value: true},
		{Key: "$or", Value: []bson.D{
			{{Key: "expires_at", Value: bson.D{{Key: "$gt", Value: time.Now()}}}},
			{{Key: "expires_at", Value: nil}},
		}},
	}).Decode(&urlData)

	if err == nil {
		// Found in main collection: update analytics and redirect
		clientIP := getClientIP(r)
		update := bson.D{
			{Key: "$inc", Value: bson.D{{Key: "clicks", Value: 1}}},
			{Key: "$set", Value: bson.D{{Key: "last_clicked", Value: time.Now().UTC()}}},
			{Key: "$push", Value: bson.D{{Key: "click_history", Value: ClickHistory{
				Timestamp: time.Now().UTC(),
				IP:        clientIP,
				UserAgent: r.Header.Get("User-Agent"),
			}}}},
		}
		_, updateErr := DB.Collection.UpdateOne(ctx, bson.D{{Key: "_id", Value: urlData.ID}}, update)
		if updateErr != nil {
			log.Printf("error updating analytics: %v", updateErr)
		}
		logSecurityEvent("URL_REDIRECT", urlData.UserID, clientIP, r.UserAgent(),
			"Redirect: "+shortURL+" -> "+urlData.LongURL, "INFO")
		log.Printf("Analytics: Short URL %s clicked, total clicks: %d", shortURL, urlData.Clicks+1)
		addSecurityHeaders(w)
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		if !validateURL(urlData.LongURL) {
			logSecurityEvent("MALICIOUS_URL_BLOCKED", urlData.UserID, clientIP, r.UserAgent(),
				"Malicious URL blocked: "+urlData.LongURL, "CRITICAL")
			http.Error(w, "URL blocked for security reasons", http.StatusForbidden)
			return
		}
		http.Redirect(w, r, urlData.LongURL, http.StatusMovedPermanently)
		return
	}

	// 2. If not found, try demo_urls collection (anonymous/demo users)
	demoCollection := DB.Database.Collection("demo_urls")
	var demoURL struct {
		LongURL   string    `bson:"long_url"`
		ExpiresAt time.Time `bson:"expires_at"`
	}
	err = demoCollection.FindOne(ctx, bson.M{
		"short_url":  shortURL,
		"expires_at": bson.M{"$gt": time.Now()},
	}).Decode(&demoURL)
	if err == nil {
		// Found in demo collection: just redirect (no analytics)
		addSecurityHeaders(w)
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		if !validateURL(demoURL.LongURL) {
			logSecurityEvent("MALICIOUS_URL_BLOCKED", "", getClientIP(r), r.UserAgent(),
				"Malicious URL blocked: "+demoURL.LongURL, "CRITICAL")
			http.Error(w, "URL blocked for security reasons", http.StatusForbidden)
			return
		}
		http.Redirect(w, r, demoURL.LongURL, http.StatusMovedPermanently)
		return
	}

	// Not found in either collection
	log.Printf("Short URL not found or expired: %s", shortURL)
	logSecurityEvent("URL_NOT_FOUND", "", getClientIP(r), r.UserAgent(),
		"URL not found: "+shortURL, "INFO")
	http.NotFound(w, r)
}

// ============================================================================
// BULK UPLOAD HANDLERS
// ============================================================================

// bulkShorten handles POST /bulk requests for bulk URL creation
func bulkShorten(w http.ResponseWriter, r *http.Request) {
	clientIP := getClientIP(r)

	// Validate request method
	if r.Method != http.MethodPost {
		logSecurityEvent("INVALID_METHOD", "", clientIP, r.UserAgent(),
			"Invalid method for bulk upload: "+r.Method, "WARN")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract user ID from JWT context
	userID, ok := r.Context().Value("user_id").(string)
	if !ok {
		logSecurityEvent("UNAUTHORIZED_BULK_ACCESS", "", clientIP, r.UserAgent(),
			"Unauthorized bulk upload attempt", "WARN")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse multipart form data with size limit (10MB)
	err := r.ParseMultipartForm(10 << 20) // 10MB max
	if err != nil {
		logSecurityEvent("BULK_UPLOAD_ERROR", userID, clientIP, r.UserAgent(),
			"Failed to parse multipart form: "+err.Error(), "ERROR")
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}

	// Get uploaded file
	file, header, err := r.FormFile("file")
	if err != nil {
		logSecurityEvent("BULK_UPLOAD_ERROR", userID, clientIP, r.UserAgent(),
			"No file uploaded: "+err.Error(), "WARN")
		http.Error(w, "No file uploaded", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Validate file
	if err := validateUploadedFile(header); err != nil {
		logSecurityEvent("BULK_UPLOAD_ERROR", userID, clientIP, r.UserAgent(),
			"Invalid file: "+err.Error(), "WARN")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Log bulk upload start
	logSecurityEvent("BULK_UPLOAD_START", userID, clientIP, r.UserAgent(),
		fmt.Sprintf("Processing file: %s (%.2f KB)", header.Filename, float64(header.Size)/1024), "INFO")

	// Process the file
	results, err := processBulkFile(file, header, userID, clientIP, r.UserAgent())
	if err != nil {
		logSecurityEvent("BULK_UPLOAD_ERROR", userID, clientIP, r.UserAgent(),
			"Failed to process file: "+err.Error(), "ERROR")
		http.Error(w, fmt.Sprintf("Failed to process file: %v", err), http.StatusInternalServerError)
		return
	}

	// Log completion
	logSecurityEvent("BULK_UPLOAD_COMPLETE", userID, clientIP, r.UserAgent(),
		fmt.Sprintf("Processed %d URLs, %d successful, %d failed",
			results.TotalProcessed, results.Successful, results.Failed), "INFO")

	// Return results
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(results)
}

// validateUploadedFile validates file type and size
func validateUploadedFile(header *multipart.FileHeader) error {
	// Check file size (10MB limit)
	if header.Size > 10<<20 {
		return fmt.Errorf("file too large. Maximum size: 10MB (current: %.2f MB)",
			float64(header.Size)/(1024*1024))
	}

	// Check file extension
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".csv" {
		return fmt.Errorf("invalid file type. Only CSV files are supported (got: %s)", ext)
	}

	return nil
}

// processBulkFile processes the uploaded file and creates URLs
func processBulkFile(file multipart.File, header *multipart.FileHeader, userID, clientIP, userAgent string) (*BulkResponse, error) {
	startTime := time.Now()

	// Parse CSV file
	urls, err := parseCSVFile(file)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CSV: %v", err)
	}

	if len(urls) == 0 {
		return nil, fmt.Errorf("no valid URLs found in file")
	}

	// Limit number of URLs to process (prevent abuse)
	const maxURLsPerBatch = 1000
	if len(urls) > maxURLsPerBatch {
		return nil, fmt.Errorf("too many URLs in file. Maximum allowed: %d (found: %d)",
			maxURLsPerBatch, len(urls))
	}

	// Process URLs concurrently with goroutines
	results := make([]BulkURLResult, len(urls))
	successful := 0
	failed := 0

	// Use worker pool pattern for controlled concurrency
	const maxWorkers = 10
	jobs := make(chan int, len(urls))
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Start workers
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				result := processSingleURL(urls[index], userID, clientIP, userAgent)

				mu.Lock()
				results[index] = result
				if result.Success {
					successful++
				} else {
					failed++
				}
				mu.Unlock()
			}
		}()
	}

	// Send jobs to workers
	for i := range urls {
		jobs <- i
	}
	close(jobs)

	// Wait for all workers to complete
	wg.Wait()

	processingTime := time.Since(startTime)

	return &BulkResponse{
		TotalProcessed: len(urls),
		Successful:     successful,
		Failed:         failed,
		Results:        results,
		ProcessingTime: processingTime.String(),
	}, nil
}

// parseCSVFile parses CSV file and returns slice of BulkURLRequest
func parseCSVFile(file multipart.File) ([]BulkURLRequest, error) {
	// Reset file pointer to beginning
	file.Seek(0, io.SeekStart)

	reader := csv.NewReader(file)
	reader.TrimLeadingSpace = true

	// Read all records
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV: %v", err)
	}

	if len(records) < 2 {
		return nil, fmt.Errorf("CSV must contain header row and at least one data row")
	}

	// Validate header (first row)
	header := records[0]
	if len(header) == 0 || strings.TrimSpace(header[0]) == "" {
		return nil, fmt.Errorf("invalid CSV header: first column must be 'Long URL'")
	}

	// Parse data rows
	var urls []BulkURLRequest
	for _, record := range records[1:] {
		// Skip empty rows
		if len(record) == 0 || strings.TrimSpace(record[0]) == "" {
			continue
		}

		url := BulkURLRequest{
			LongURL: strings.TrimSpace(record[0]),
		}

		// Validate required field
		if url.LongURL == "" {
			continue // Skip rows without URL
		}

		// Parse optional fields
		if len(record) > 1 && strings.TrimSpace(record[1]) != "" {
			url.Domain = strings.TrimSpace(record[1])
		}
		if len(record) > 2 && strings.TrimSpace(record[2]) != "" {
			url.CustomAlias = strings.TrimSpace(record[2])
		}
		if len(record) > 3 && strings.TrimSpace(record[3]) != "" {
			tagString := strings.TrimSpace(record[3])
			tags := strings.Split(tagString, ";")
			var cleanTags []string
			for _, tag := range tags {
				cleaned := strings.TrimSpace(tag)
				if cleaned != "" {
					cleanTags = append(cleanTags, cleaned)
				}
			}
			url.Tags = cleanTags
		}
		if len(record) > 4 && strings.TrimSpace(record[4]) != "" {
			url.Expires = strings.TrimSpace(record[4])
		}

		urls = append(urls, url)
	}

	return urls, nil
}

// processSingleURL processes a single URL and returns the result
func processSingleURL(req BulkURLRequest, userID, clientIP, userAgent string) BulkURLResult {
	result := BulkURLResult{
		LongURL: req.LongURL,
		Domain:  req.Domain,
		Tags:    req.Tags,
	}

	// Validate URL
	if !validateURL(req.LongURL) {
		result.Error = "Invalid URL format"
		return result
	}

	// Set default domain if not provided
	if req.Domain == "" {
		req.Domain = os.Getenv("BASE_URL")
		if req.Domain == "" {
			req.Domain = "http://localhost:8080"
		}
		result.Domain = req.Domain
	}

	// Sanitize tags
	if len(req.Tags) > 0 {
		req.Tags = sanitizeStringSlice(req.Tags)
		result.Tags = req.Tags
	}

	// Check for existing URL to avoid duplicates
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var existingURL URLData
	err := DB.Collection.FindOne(ctx, bson.D{
		{Key: "long_url", Value: req.LongURL},
		{Key: "domain", Value: req.Domain},
		{Key: "user_id", Value: userID},
		{Key: "is_active", Value: true},
	}).Decode(&existingURL)

	if err == nil {
		// URL already exists, return existing
		result.ShortURL = existingURL.ShortURL
		result.Success = true
		result.CreatedAt = existingURL.CreatedAt.Format(time.RFC3339)
		return result
	}

	// Generate new short URL
	shortCode, err := generateShortCodeForBulk(req.LongURL, req.CustomAlias)
	if err != nil {
		result.Error = fmt.Sprintf("Failed to generate short code: %v", err)
		return result
	}

	// Parse expiration if provided
	var expiresAt *time.Time
	if req.Expires != "" {
		if parsed, err := time.Parse(time.RFC3339, req.Expires); err == nil {
			expiresAt = &parsed
		} else if parsed, err := time.Parse("2006-01-02", req.Expires); err == nil {
			// Set to end of day for date-only format
			endOfDay := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 23, 59, 59, 999999999, parsed.Location())
			expiresAt = &endOfDay
		} else {
			result.Error = fmt.Sprintf("Invalid expiration date format: %s (use YYYY-MM-DD or RFC3339)", req.Expires)
			return result
		}
	} else {
		// Default to 5 years
		defaultExpiry := time.Now().AddDate(5, 0, 0)
		expiresAt = &defaultExpiry
	}

	// Create URL document
	urlData := URLData{
		ID:           primitive.NewObjectID(),
		ShortURL:     shortCode,
		LongURL:      req.LongURL,
		Domain:       req.Domain,
		Tags:         req.Tags,
		UserID:       userID,
		CreatedAt:    time.Now().UTC(),
		ExpiresAt:    expiresAt,
		Clicks:       0,
		IsActive:     true,
		ClickHistory: []ClickHistory{},
	}

	// Insert into database
	_, err = DB.Collection.InsertOne(ctx, urlData)
	if err != nil {
		result.Error = fmt.Sprintf("Database error: %v", err)
		return result
	}

	result.ShortURL = shortCode
	result.Success = true
	result.CreatedAt = urlData.CreatedAt.Format(time.RFC3339)

	return result
}

// generateShortCodeForBulk generates short code for bulk processing
func generateShortCodeForBulk(longURL, customAlias string) (string, error) {
	if customAlias != "" {
		// Validate custom alias
		if !validateCustomURL(customAlias) {
			return "", fmt.Errorf("invalid custom alias format")
		}

		// Check if custom alias already exists
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var existing URLData
		err := DB.Collection.FindOne(ctx, bson.D{
			{Key: "short_url", Value: customAlias},
			{Key: "is_active", Value: true},
		}).Decode(&existing)

		if err == nil {
			return "", fmt.Errorf("custom alias '%s' already exists", customAlias)
		}

		return customAlias, nil
	}

	// Generate using existing logic
	code := generateReadableCode(longURL)
	return code, nil
}

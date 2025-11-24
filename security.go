package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"html"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// ============================================================================
// ENCRYPTION UTILITIES
// ============================================================================

var encryptionKey []byte

// InitEncryption initializes the encryption key from environment
func InitEncryption() error {
	key := os.Getenv("ENCRYPTION_KEY")
	if key == "" {
		// Generate a random 32-byte key if not provided (development only)
		encryptionKey = make([]byte, 32)
		if _, err := rand.Read(encryptionKey); err != nil {
			return err
		}
		return nil
	}

	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil || len(decoded) != 32 {
		return errors.New("ENCRYPTION_KEY must be a base64-encoded 32-byte key")
	}
	encryptionKey = decoded
	return nil
}

// EncryptSensitiveData encrypts sensitive information using AES-256-GCM
func EncryptSensitiveData(plaintext string) (string, error) {
	if len(encryptionKey) != 32 {
		return "", errors.New("encryption not initialized")
	}

	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptSensitiveData decrypts sensitive information
func DecryptSensitiveData(ciphertext string) (string, error) {
	if len(encryptionKey) != 32 {
		return "", errors.New("encryption not initialized")
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, cipherData := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, cipherData, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// ============================================================================
// INPUT SANITIZATION UTILITIES
// ============================================================================

// sanitizeInput removes XSS vectors and dangerous characters
func sanitizeInput(input string) string {
	// Remove any HTML/script tags to prevent XSS
	input = html.EscapeString(input)

	// Remove null bytes and control characters
	input = strings.ReplaceAll(input, "\x00", "")

	// Remove other control characters except newlines and tabs
	var result strings.Builder
	for _, r := range input {
		if r == '\n' || r == '\t' || r == '\r' || (r >= 32 && r != 127) {
			result.WriteRune(r)
		}
	}

	// Trim whitespace
	return strings.TrimSpace(result.String())
}

// validateEmail validates email format and length
func validateEmail(email string) bool {
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	return emailRegex.MatchString(email) && len(email) <= 254 && utf8.ValidString(email)
}

// validateUsername validates username format
func validateUsername(username string) bool {
	// Only allow alphanumeric and safe special characters
	usernameRegex := regexp.MustCompile(`^[A-Za-z]+(?:[ .-][A-Za-z]+)*$`)
	return usernameRegex.MatchString(username) && utf8.ValidString(username)
}

// validateURL validates URL format and security
func validateURL(longURL string) bool {
	// Parse and validate URL
	parsedURL, err := url.Parse(longURL)
	if err != nil {
		return false
	}

	// Check if scheme is HTTP or HTTPS only
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return false
	}

	// Check URL length (max 2048 characters)
	if len(longURL) > 2048 || len(longURL) < 10 {
		return false
	}

	// Check for valid hostname
	if parsedURL.Host == "" {
		return false
	}

	// Prevent localhost and internal IPs (configurable via environment)
	hostname := strings.ToLower(parsedURL.Host)
	allowLocalhost := os.Getenv("ALLOW_LOCALHOST") == "true"

	if (!allowLocalhost && strings.Contains(hostname, "localhost")) ||
		strings.Contains(hostname, "127.0.0.1") ||
		strings.Contains(hostname, "0.0.0.0") ||
		strings.HasPrefix(hostname, "192.168.") ||
		strings.HasPrefix(hostname, "10.") {
		return false
	}

	// Validate UTF-8
	if !utf8.ValidString(longURL) {
		return false
	}

	return true
}

// validatePassword validates password strength
func validatePassword(password string) bool {
	// Length check (8-128 characters)
	if len(password) < 8 || len(password) > 128 {
		return false
	}

	// UTF-8 validation
	if !utf8.ValidString(password) {
		return false
	}

	// Must contain at least one letter and one number
	hasLetter := regexp.MustCompile(`[a-zA-Z]`).MatchString(password)
	hasNumber := regexp.MustCompile(`[0-9]`).MatchString(password)

	return hasLetter && hasNumber
}

// validateCustomURL validates custom short URL format
func validateCustomURL(custom string) bool {
	if custom == "" {
		return true // Optional field
	}

	// Only alphanumeric characters, hyphens, and underscores
	customRegex := regexp.MustCompile(`^[a-zA-Z0-9_-]{3,20}$`)
	return customRegex.MatchString(custom) && utf8.ValidString(custom)
}

// ============================================================================
// SECURITY HEADERS AND UTILITIES
// ============================================================================

// addSecurityHeaders adds comprehensive security headers to response
func addSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-XSS-Protection", "1; mode=block")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'")
	w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
}

// getClientIP safely extracts client IP address
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (behind proxy)
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		// Take first IP if multiple (most trusted)
		ips := strings.Split(forwarded, ",")
		ip := strings.TrimSpace(ips[0])
		if ip != "" {
			return ip
		}
	}

	// Check X-Real-IP header
	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		return realIP
	}

	// Fallback to RemoteAddr
	ip := r.RemoteAddr
	if strings.Contains(ip, ":") {
		// Remove port if present
		if lastColon := strings.LastIndex(ip, ":"); lastColon != -1 {
			ip = ip[:lastColon]
		}
	}

	// Remove brackets if IPv6
	ip = strings.Trim(ip, "[]")

	return ip
}

// isValidContentType validates request content type for security
func isValidContentType(contentType string) bool {
	allowedTypes := []string{
		"application/json",
		"application/x-www-form-urlencoded",
		"multipart/form-data",
	}

	for _, allowed := range allowedTypes {
		if strings.Contains(strings.ToLower(contentType), allowed) {
			return true
		}
	}
	return false
}

// ============================================================================
// SECURITY LOGGING
// ============================================================================

// SecurityEvent represents a security-related event
type SecurityEvent struct {
	Timestamp string `json:"timestamp" bson:"timestamp"`
	Event     string `json:"event" bson:"event"`
	UserID    string `json:"user_id,omitempty" bson:"user_id,omitempty"`
	IP        string `json:"ip" bson:"ip"`
	UserAgent string `json:"user_agent,omitempty" bson:"user_agent,omitempty"`
	Details   string `json:"details,omitempty" bson:"details,omitempty"`
	Severity  string `json:"severity" bson:"severity"` // INFO, WARN, ERROR, CRITICAL
}

// logSecurityEvent logs security events asynchronously
func logSecurityEvent(event, userID, ip, userAgent, details, severity string) {
	go func() {
		// Log to console for now (can be extended to database/external service)
		log.Printf("🔒 SECURITY [%s] %s - %s (IP: %s, User: %s)",
			severity, event, details, ip, userID)

		// TODO: Store in security events collection if database is available
		// if DB != nil && DB.Collection != nil {
		//     securityEvent := SecurityEvent{
		//         Timestamp: time.Now().UTC().Format(time.RFC3339),
		//         Event:     event,
		//         UserID:    userID,
		//         IP:        ip,
		//         UserAgent: userAgent,
		//         Details:   details,
		//         Severity:  severity,
		//     }
		//     DB.Database.Collection("security_events").InsertOne(context.TODO(), securityEvent)
		// }
	}()
}

// ============================================================================
// RATE LIMITING (INFRASTRUCTURE)
// ============================================================================

// RateLimitInfo holds rate limiting information per IP/User
type RateLimitInfo struct {
	LastRequest  time.Time `json:"last_request"`
	RequestCount int       `json:"request_count"`
	WindowStart  time.Time `json:"window_start"`
}

// Global rate limiting maps (in production, use Redis or similar)
var (
	ipRateLimits   = make(map[string]*RateLimitInfo)
	rateLimitMutex = sync.RWMutex{}
)

// checkRateLimit checks if request should be rate limited (basic implementation)
func checkRateLimit(identifier string, maxRequests int, windowDuration time.Duration) bool {
	rateLimitMutex.Lock()
	defer rateLimitMutex.Unlock()

	now := time.Now()
	info, exists := ipRateLimits[identifier]

	if !exists {
		ipRateLimits[identifier] = &RateLimitInfo{
			LastRequest:  now,
			RequestCount: 1,
			WindowStart:  now,
		}
		return false // Allow first request
	}

	// Reset window if expired
	if now.Sub(info.WindowStart) > windowDuration {
		info.RequestCount = 1
		info.WindowStart = now
		info.LastRequest = now
		return false
	}

	// Check if limit exceeded
	if info.RequestCount >= maxRequests {
		return true // Rate limited
	}

	info.RequestCount++
	info.LastRequest = now
	return false
}

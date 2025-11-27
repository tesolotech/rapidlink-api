package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables from .env file
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found, using system environment variables: %v", err)
	}

	// Verify critical environment variables
	if baseURL := os.Getenv("BASE_URL"); baseURL == "" {
		log.Println("⚠️  BASE_URL not set, using default: http://localhost:8080")
		os.Setenv("BASE_URL", "http://localhost:8080")
	} else {
		log.Printf("✅ BASE_URL loaded: %s", baseURL)
	}

	// Initialize encryption for sensitive data
	if err := InitEncryption(); err != nil {
		log.Fatalf("❌ Encryption initialization failed: %v", err)
	}
	log.Println("✅ Encryption initialized successfully!")

	// Initialize MongoDB connection
	if err := InitializeDatabase(); err != nil {
		log.Fatalf("❌ %v", err)
	}
	defer CloseMongoDB()

	// Ensure TTL index for demo_urls
	if err := EnsureDemoURLTTLIndex(); err != nil {
		log.Fatalf("❌ Failed to ensure TTL index for demo_urls: %v", err)
	}

	// Initialize JWT
	InitJWT()
	log.Println("✅ JWT initialized successfully!")

	// Start cleanup worker for expired URLs
	StartCleanupWorker()

	// Create router with Gorilla Mux for better performance
	r := mux.NewRouter()

	// Add security middleware
	r.Use(securityMiddleware)

	// Authentication routes (public)
	authRouter := r.PathPrefix("/auth").Subrouter()
	authRouter.HandleFunc("/register", register).Methods("POST")
	authRouter.HandleFunc("/login", login).Methods("POST")
	authRouter.HandleFunc("/validate", validateToken).Methods("POST")
	authRouter.HandleFunc("/refresh", refreshTokenHandler).Methods("POST")

	// Protected authentication route
	authRouter.HandleFunc("/profile", JWTMiddleware(profile)).Methods("GET")

	// Protected URL shortening endpoint
	r.HandleFunc("/url", JWTMiddleware(shorten)).Methods("PUT")
	// Protected URL delete endpoint
	r.HandleFunc("/url", JWTMiddleware(deleteShortURL)).Methods("DELETE")

	// Protected bulk upload endpoint
	r.HandleFunc("/bulk", JWTMiddleware(bulkShorten)).Methods("POST")

	// Protected analytics endpoint
	r.HandleFunc("/analytics", JWTMiddleware(analytics)).Methods("GET")

	// Public demo shortener endpoints
	r.HandleFunc("/rapidlink-demo", rapidLinkDemo).Methods("PUT")
	r.HandleFunc("/rapidlink-demo", getDemoURLs).Methods("GET")

	// Catch-all route to handle redirect via short_url
	// This must be last to avoid conflicts
	r.PathPrefix("/").HandlerFunc(redirect).Methods("GET")

	// Add compression middleware for better performance
	compressedHandler := handlers.CompressHandler(r)

	// Add CORS middleware for cross-origin requests (production: restrict origins)
	allowedOrigins := []string{"*"} // TODO: Restrict in production
	if corsOrigins := os.Getenv("ALLOWED_ORIGINS"); corsOrigins != "" {
		allowedOrigins = strings.Split(corsOrigins, ",")
	}

	corsHandler := handlers.CORS(
		handlers.AllowedOrigins(allowedOrigins),
		handlers.AllowedMethods([]string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}),
		handlers.AllowedHeaders([]string{"Content-Type", "Authorization"}),
		handlers.AllowCredentials(),
	)(compressedHandler)

	// Add request logging middleware
	loggedHandler := handlers.LoggingHandler(os.Stdout, corsHandler)

	// Configure server with optimized settings
	server := &http.Server{
		Addr:           ":8080",
		Handler:        loggedHandler,
		ReadTimeout:    15 * time.Second, // Time to read request
		WriteTimeout:   15 * time.Second, // Time to write response
		IdleTimeout:    60 * time.Second, // Time to keep connections alive
		MaxHeaderBytes: 1 << 20,          // Max header size (1MB)
	}

	// Start server in a goroutine
	go func() {
		log.Println("🚀 Server starting...")
		log.Println("🔒 Security features enabled:")
		log.Println("   ✓ JWT Authentication")
		log.Println("   ✓ Input Sanitization (XSS Protection)")
		log.Println("   ✓ Parameterized Queries (Injection Protection)")
		log.Println("   ✓ Data Encryption (AES-256-GCM)")
		log.Println("   ✓ Principle of Least Privilege")
		log.Println("   ✓ Security Headers")
		log.Println("   ✓ Rate Limiting Infrastructure")
		log.Println("")
		log.Println("📋 Available endpoints:")
		log.Println("   Public:")
		log.Println("     POST /auth/register - Create new user account")
		log.Println("     POST /auth/login - Login and get JWT token")
		log.Println("     POST /auth/validate - Validate JWT token")
		log.Println("     GET  /<short-url> - Redirect to long URL")
		log.Println("   Protected (requires Bearer token):")
		log.Println("     GET  /auth/profile - Get user profile")
		log.Println("     PUT  /url - Create short URL")
		log.Println("     POST /bulk - Bulk create short URLs from CSV")
		log.Println("     GET  /analytics - Get URL analytics")
		log.Println("")
		log.Printf("🌐 Server running on http://localhost%s", server.Addr)
		log.Printf("🔧 Features: Compression ✓ | CORS ✓ | Request Logging ✓ | Graceful Shutdown ✓")
		log.Printf("⚡ Optimizations: Connection Pool ✓ | Timeouts ✓ | Performance Routing ✓")
		log.Printf("🛡️  Security: Input Validation ✓ | Encryption ✓ | Headers ✓ | Rate Limiting Ready ✓")
		log.Println("")

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// Block until we receive our signal
	<-c

	// Create a deadline to wait for graceful shutdown
	log.Println("🛑 Interrupt signal received, shutting down gracefully...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Attempt graceful shutdown
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	// Close database connection
	CloseMongoDB()
	log.Println("✅ Server stopped gracefully")
}

// securityMiddleware adds security headers and validation to all requests
func securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add security headers to all responses
		addSecurityHeaders(w)

		// Validate Content-Type for POST/PUT requests
		if r.Method == "POST" || r.Method == "PUT" {
			contentType := r.Header.Get("Content-Type")
			if !isValidContentType(contentType) {
				logSecurityEvent("INVALID_CONTENT_TYPE", "", getClientIP(r), r.UserAgent(),
					"Invalid content type: "+contentType, "WARN")
				http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
				return
			}
		}

		// Basic rate limiting check (can be enhanced)
		clientIP := getClientIP(r)
		if checkRateLimit(clientIP, 100, time.Minute) {
			logSecurityEvent("RATE_LIMIT_EXCEEDED", "", clientIP, r.UserAgent(),
				"Rate limit exceeded", "WARN")
			http.Error(w, "Rate limit exceeded. Please try again later.", http.StatusTooManyRequests)
			return
		}

		// Log security events for sensitive endpoints
		if r.Method == "POST" && (strings.Contains(r.URL.Path, "/auth/") || strings.Contains(r.URL.Path, "/url")) {
			logSecurityEvent("API_ACCESS", "", clientIP, r.UserAgent(),
				r.Method+" "+r.URL.Path, "INFO")
		}

		next.ServeHTTP(w, r)
	})
}

package main

// Centralized constants for RapidLink backend

const (
	// Application
	AppName           = "RapidLink"
	DefaultPort       = ":8080"
	DefaultBaseURL    = "http://localhost:8080"
	DefaultDomain     = "http://localhost:8080"
	DefaultTokenTTL   = 24 * 60 * 60     // 24 hours in seconds
	RefreshTokenTTL   = 7 * 24 * 60 * 60 // 7 days in seconds
	MaxBulkUploadSize = 10 * 1024 * 1024 // 10MB
)

var (
	// Default domains for dropdowns or validation
	DefaultDomains = []string{
		"http://localhost:8080",
		"http://rapidlink.com",
	}

	// Default tags for new links
	DefaultTags = []string{
		"Education",
		"Technology",
		"Science",
		"Health",
	}
)

// Add more constants as needed for your application

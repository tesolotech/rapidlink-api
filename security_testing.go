package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Security Testing Suite
func mainSecurity() {
	baseURL := "http://localhost:8080"

	fmt.Println("🔒 COMPREHENSIVE SECURITY TESTING SUITE")
	fmt.Println("=======================================")
	fmt.Println()

	// Test 1: Input Sanitization (XSS Protection)
	fmt.Println("🛡️ Test 1: Input Sanitization & XSS Protection")
	fmt.Println("-----------------------------------------------")
	testInputSanitization(baseURL)

	// Test 2: Authentication Security
	fmt.Println("\n🔐 Test 2: Authentication Security")
	fmt.Println("----------------------------------")
	testAuthenticationSecurity(baseURL)

	// Test 3: URL Validation Security
	fmt.Println("\n🌐 Test 3: URL Validation Security")
	fmt.Println("----------------------------------")
	testURLValidation(baseURL)

	// Test 4: Security Headers
	fmt.Println("\n📋 Test 4: Security Headers")
	fmt.Println("---------------------------")
	testSecurityHeaders(baseURL)

	// Test 5: Rate Limiting
	fmt.Println("\n⚡ Test 5: Rate Limiting")
	fmt.Println("-----------------------")
	testRateLimiting(baseURL)

	// Test 6: Content Type Validation
	fmt.Println("\n📝 Test 6: Content Type Validation")
	fmt.Println("----------------------------------")
	testContentTypeValidation(baseURL)

	// Test 7: Malicious Payload Protection
	fmt.Println("\n☠️ Test 7: Malicious Payload Protection")
	fmt.Println("---------------------------------------")
	testMaliciousPayloads(baseURL)

	fmt.Println("\n🎯 SECURITY TESTING COMPLETE")
	fmt.Println("=============================")
}

func testInputSanitization(baseURL string) {
	maliciousInputs := []struct {
		name          string
		payload       map[string]interface{}
		expectBlocked bool
	}{
		{
			name: "XSS in Username",
			payload: map[string]interface{}{
				"username": "<script>alert('XSS')</script>",
				"email":    "test@example.com",
				"password": "password123",
			},
			expectBlocked: true,
		},
		{
			name: "SQL Injection in Email",
			payload: map[string]interface{}{
				"username": "testuser",
				"email":    "'; DROP TABLE users; --",
				"password": "password123",
			},
			expectBlocked: true,
		},
		{
			name: "XSS in Password",
			payload: map[string]interface{}{
				"username": "testuser2",
				"email":    "test2@example.com",
				"password": "<img src=x onerror=alert('XSS')>",
			},
			expectBlocked: true,
		},
		{
			name: "Invalid Characters in Username",
			payload: map[string]interface{}{
				"username": "test\x00user\x01",
				"email":    "test3@example.com",
				"password": "password123",
			},
			expectBlocked: true,
		},
	}

	for _, test := range maliciousInputs {
		fmt.Printf("  Testing %s... ", test.name)

		jsonData, _ := json.Marshal(test.payload)
		resp, err := http.Post(baseURL+"/auth/register", "application/json", bytes.NewBuffer(jsonData))

		if err != nil {
			fmt.Printf("❌ Request failed: %v\n", err)
			continue
		}
		defer resp.Body.Close()

		if test.expectBlocked && resp.StatusCode >= 400 {
			fmt.Printf("✅ Correctly blocked (Status: %d)\n", resp.StatusCode)
		} else if test.expectBlocked && resp.StatusCode < 400 {
			fmt.Printf("❌ Should have been blocked but wasn't (Status: %d)\n", resp.StatusCode)
		} else {
			fmt.Printf("ℹ️ Status: %d\n", resp.StatusCode)
		}

		time.Sleep(100 * time.Millisecond)
	}
}

func testAuthenticationSecurity(baseURL string) {
	tests := []struct {
		name         string
		payload      map[string]interface{}
		expectStatus int
	}{
		{
			name: "Weak Password",
			payload: map[string]interface{}{
				"username": "weakuser",
				"email":    "weak@example.com",
				"password": "123",
			},
			expectStatus: 400,
		},
		{
			name: "Invalid Email Format",
			payload: map[string]interface{}{
				"username": "invaliduser",
				"email":    "not-an-email",
				"password": "password123",
			},
			expectStatus: 400,
		},
		{
			name: "Invalid Username Format",
			payload: map[string]interface{}{
				"username": "a", // Too short
				"email":    "short@example.com",
				"password": "password123",
			},
			expectStatus: 400,
		},
		{
			name: "Missing Fields",
			payload: map[string]interface{}{
				"username": "incomplete",
			},
			expectStatus: 400,
		},
	}

	for _, test := range tests {
		fmt.Printf("  Testing %s... ", test.name)

		jsonData, _ := json.Marshal(test.payload)
		resp, err := http.Post(baseURL+"/auth/register", "application/json", bytes.NewBuffer(jsonData))

		if err != nil {
			fmt.Printf("❌ Request failed: %v\n", err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == test.expectStatus {
			fmt.Printf("✅ Correct status %d\n", resp.StatusCode)
		} else {
			fmt.Printf("❌ Expected %d, got %d\n", test.expectStatus, resp.StatusCode)
		}

		time.Sleep(100 * time.Millisecond)
	}
}

func testURLValidation(baseURL string) {
	// First, register a test user and get a token
	registerPayload := map[string]interface{}{
		"username": "urltest_" + fmt.Sprint(time.Now().Unix()),
		"email":    fmt.Sprintf("urltest_%d@example.com", time.Now().Unix()),
		"password": "password123",
	}

	jsonData, _ := json.Marshal(registerPayload)
	resp, err := http.Post(baseURL+"/auth/register", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("❌ Failed to register test user: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var authResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&authResp)
	token := authResp["token"].(string)

	maliciousURLs := []struct {
		name          string
		url           string
		expectBlocked bool
	}{
		{
			name:          "Localhost URL",
			url:           "http://localhost:3000/malicious",
			expectBlocked: true,
		},
		{
			name:          "Internal IP",
			url:           "http://192.168.1.1/internal",
			expectBlocked: true,
		},
		{
			name:          "Loopback IP",
			url:           "http://127.0.0.1/dangerous",
			expectBlocked: true,
		},
		{
			name:          "Non-HTTP Scheme",
			url:           "file:///etc/passwd",
			expectBlocked: true,
		},
		{
			name:          "JavaScript Protocol",
			url:           "javascript:alert('XSS')",
			expectBlocked: true,
		},
		{
			name:          "Data URL",
			url:           "data:text/html,<script>alert('XSS')</script>",
			expectBlocked: true,
		},
		{
			name:          "Valid HTTPS URL",
			url:           "https://www.google.com",
			expectBlocked: false,
		},
	}

	for _, test := range maliciousURLs {
		fmt.Printf("  Testing %s... ", test.name)

		urlPayload := map[string]interface{}{
			"long-url": test.url,
		}

		jsonData, _ := json.Marshal(urlPayload)
		req, _ := http.NewRequest("PUT", baseURL+"/url", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		client := &http.Client{}
		resp, err := client.Do(req)

		if err != nil {
			fmt.Printf("❌ Request failed: %v\n", err)
			continue
		}
		defer resp.Body.Close()

		if test.expectBlocked && resp.StatusCode >= 400 {
			fmt.Printf("✅ Correctly blocked (Status: %d)\n", resp.StatusCode)
		} else if test.expectBlocked && resp.StatusCode < 400 {
			fmt.Printf("❌ Should have been blocked but wasn't (Status: %d)\n", resp.StatusCode)
		} else if !test.expectBlocked && resp.StatusCode < 400 {
			fmt.Printf("✅ Correctly allowed (Status: %d)\n", resp.StatusCode)
		} else {
			fmt.Printf("⚠️ Unexpected result (Status: %d)\n", resp.StatusCode)
		}

		time.Sleep(100 * time.Millisecond)
	}
}

func testSecurityHeaders(baseURL string) {
	resp, err := http.Get(baseURL + "/")
	if err != nil {
		fmt.Printf("❌ Failed to test headers: %v\n", err)
		return
	}
	defer resp.Body.Close()

	expectedHeaders := map[string]string{
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":           "DENY",
		"X-XSS-Protection":          "1; mode=block",
		"Referrer-Policy":           "strict-origin-when-cross-origin",
		"Content-Security-Policy":   "default-src 'self'",
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
		"Permissions-Policy":        "geolocation=(), microphone=(), camera=()",
	}

	for header, expectedValue := range expectedHeaders {
		actualValue := resp.Header.Get(header)
		fmt.Printf("  %s: ", header)

		if strings.Contains(actualValue, strings.Split(expectedValue, ";")[0]) {
			fmt.Printf("✅ Present\n")
		} else {
			fmt.Printf("❌ Missing or incorrect (got: %s)\n", actualValue)
		}
	}
}

func testRateLimiting(baseURL string) {
	fmt.Printf("  Testing rate limiting with rapid requests... ")

	successCount := 0
	rateLimitedCount := 0

	for i := 0; i < 10; i++ {
		resp, err := http.Get(baseURL + "/")
		if err != nil {
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == 429 {
			rateLimitedCount++
		} else if resp.StatusCode < 400 {
			successCount++
		}

		// No delay to test rapid requests
	}

	fmt.Printf("✅ %d successful, %d rate limited\n", successCount, rateLimitedCount)
	if rateLimitedCount > 0 {
		fmt.Printf("  ℹ️ Rate limiting is working\n")
	}
}

func testContentTypeValidation(baseURL string) {
	tests := []struct {
		name          string
		contentType   string
		expectBlocked bool
	}{
		{
			name:          "Valid JSON",
			contentType:   "application/json",
			expectBlocked: false,
		},
		{
			name:          "Invalid Content-Type",
			contentType:   "text/plain",
			expectBlocked: true,
		},
		{
			name:          "Missing Content-Type",
			contentType:   "",
			expectBlocked: true,
		},
		{
			name:          "XML Content-Type",
			contentType:   "application/xml",
			expectBlocked: true,
		},
	}

	for _, test := range tests {
		fmt.Printf("  Testing %s... ", test.name)

		payload := map[string]interface{}{
			"username": "cttest",
			"email":    "ct@example.com",
			"password": "password123",
		}

		jsonData, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", baseURL+"/auth/register", bytes.NewBuffer(jsonData))
		if test.contentType != "" {
			req.Header.Set("Content-Type", test.contentType)
		}

		client := &http.Client{}
		resp, err := client.Do(req)

		if err != nil {
			fmt.Printf("❌ Request failed: %v\n", err)
			continue
		}
		defer resp.Body.Close()

		if test.expectBlocked && resp.StatusCode == 415 {
			fmt.Printf("✅ Correctly blocked (Status: %d)\n", resp.StatusCode)
		} else if !test.expectBlocked && resp.StatusCode != 415 {
			fmt.Printf("✅ Correctly allowed (Status: %d)\n", resp.StatusCode)
		} else {
			fmt.Printf("⚠️ Unexpected result (Status: %d)\n", resp.StatusCode)
		}

		time.Sleep(100 * time.Millisecond)
	}
}

func testMaliciousPayloads(baseURL string) {
	payloads := []struct {
		name     string
		payload  string
		endpoint string
	}{
		{
			name:     "JSON Injection",
			payload:  `{"username": "test", "email": "test@example.com", "password": "pass", "admin": true}`,
			endpoint: "/auth/register",
		},
		{
			name:     "Oversized Payload",
			payload:  `{"username": "` + strings.Repeat("A", 10000) + `", "email": "test@example.com", "password": "password123"}`,
			endpoint: "/auth/register",
		},
		{
			name:     "Null Bytes",
			payload:  "{\"username\": \"test\x00admin\", \"email\": \"test@example.com\", \"password\": \"password123\"}",
			endpoint: "/auth/register",
		},
	}

	for _, test := range payloads {
		fmt.Printf("  Testing %s... ", test.name)

		req, _ := http.NewRequest("POST", baseURL+test.endpoint, strings.NewReader(test.payload))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(req)

		if err != nil {
			fmt.Printf("❌ Request failed: %v\n", err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			fmt.Printf("✅ Correctly blocked (Status: %d)\n", resp.StatusCode)
		} else {
			fmt.Printf("⚠️ Allowed (Status: %d)\n", resp.StatusCode)
		}

		time.Sleep(100 * time.Millisecond)
	}
}

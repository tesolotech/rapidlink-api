# JWT Authentication for URL Shortener

## 🔐 **Authentication System Overview**

The URL shortener now includes JWT-based authentication with Bearer token support. All URL management endpoints require authentication.

## 📋 **API Endpoints**

### **Public Endpoints (No Authentication Required)**

#### 1. **User Registration**
```http
POST /auth/register
Content-Type: application/json

{
  "username": "johndoe",
  "email": "john@example.com",
  "password": "password123"
}
```

**Response:**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires_at": "2025-11-18T11:22:00Z",
  "user": {
    "id": "64a7b9c1234567890abcdef0",
    "username": "johndoe",
    "email": "john@example.com",
    "created_at": "2025-11-17T11:22:00Z",
    "is_active": true
  }
}
```

#### 2. **User Login**
```http
POST /auth/login
Content-Type: application/json

{
  "username_or_email": "johndoe",  // Can be username or email
  "password": "password123"
}
```

**Response:** Same as registration

#### 3. **Token Validation**
```http
POST /auth/validate
Content-Type: application/json

{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

**Response:**
```json
{
  "valid": true,
  "user_id": "64a7b9c1234567890abcdef0",
  "username": "johndoe",
  "email": "john@example.com",
  "expires": "2025-11-18T11:22:00Z"
}
```

#### 4. **URL Redirect** (Public)
```http
GET /<short-url>
```
Redirects to the original long URL (no authentication needed)

### **Protected Endpoints (Require Bearer Token)**

All protected endpoints require the `Authorization` header:
```http
Authorization: Bearer <your-jwt-token>
```

#### 5. **Get User Profile**
```http
GET /auth/profile
Authorization: Bearer <token>
```

**Response:**
```json
{
  "id": "64a7b9c1234567890abcdef0",
  "username": "johndoe",
  "email": "john@example.com",
  "created_at": "2025-11-17T11:22:00Z",
  "is_active": true
}
```

#### 6. **Create Short URL**
```http
PUT /url
Authorization: Bearer <token>
Content-Type: application/json

{
  "long-url": "https://example.com/very/long/url",
  "expires": "2025-12-31T23:59:59Z",  // Optional
  "custom": "my-custom-url"           // Optional
}
```

**Response:**
```json
{
  "long-url": "https://example.com/very/long/url",
  "short-url": "abc123",
  "user_id": "64a7b9c1234567890abcdef0",
  "created-at": "2025-11-17T11:22:00Z",
  "expires-at": "2030-11-17T11:22:00Z",
  "is-active": true
}
```

#### 7. **Get Analytics (Paginated)**
```http
GET /analytics?page=1&pageSize=20
Authorization: Bearer <token>
```

**Query Parameters:**
- `page` (optional, default: 1): Page number to retrieve
- `pageSize` (optional, default: 20, max: 100): Number of URLs per page

**Response:**
```json
{
  "success": true,
  "message": "Analytics retrieved successfully",
  "statistics": {
    "total_urls": 42,
    "total_clicks": 123,
    "avg_clicks_per_url": 2.9
  },
  "urls": [
    {
      "short_url": "abc123",
      "long_url": "https://example.com/very/long/url",
      "domain": "http://localhost:8080",
      "tags": ["Education", "Tech"],
      "clicks": 5,
      "created_at": "2025-11-17T11:22:00Z",
      "expires_at": "2030-11-17T11:22:00Z",
      "is_active": true
    }
    // ...more URLs for this page
  ],
  "page": 1,
  "pageSize": 20,
  "total": 42,
  "count": 20
}
```

**Notes:**
- The `urls` array contains only the URLs for the requested page.
- Use `page` and `pageSize` to navigate through all your URLs efficiently.
- The legacy `limit` parameter is still supported for backward compatibility, but `page` and `pageSize` are recommended.

**Example:**
```bash
curl -X GET "http://localhost:8080/analytics?page=2&pageSize=10" \
  -H "Authorization: Bearer YOUR_TOKEN_HERE"
```

## 🔧 **Configuration**

### **JWT Secret**
The JWT secret is automatically generated or can be set via environment variable:
```bash
export JWT_SECRET="your-super-secret-key-here"
```

### **Token Expiry**
Default: 24 hours. Modify `TokenDuration` in `auth.go` to change.

## 🧪 **Testing Examples**

### **1. Register a New User**
```bash
curl -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "username": "testuser",
    "email": "test@example.com",
    "password": "password123"
  }'
```

### **2. Login and Get Token**
```bash
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "username_or_email": "testuser",
    "password": "password123"
  }'
```

### **3. Create Short URL (with token)**
```bash
# Replace YOUR_TOKEN_HERE with the actual token from login
curl -X PUT http://localhost:8080/url \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN_HERE" \
  -d '{
    "long-url": "https://github.com/golang/go",
    "custom": "golang-repo"
  }'
```

### **4. Get User Profile**
```bash
curl -X GET http://localhost:8080/auth/profile \
  -H "Authorization: Bearer YOUR_TOKEN_HERE"
```

### **5. Get Analytics**
```bash
curl -X GET "http://localhost:8080/analytics?short_url=golang-repo" \
  -H "Authorization: Bearer YOUR_TOKEN_HERE"
```

## 🛡️ **Security Features**

1. **Password Hashing**: bcrypt with default cost
2. **JWT Tokens**: HMAC-SHA256 signing
3. **Token Expiry**: 24-hour default expiration
4. **User Isolation**: URLs are tied to user accounts
5. **Input Validation**: Comprehensive validation on all endpoints
6. **Error Handling**: Secure error messages (no sensitive data leakage)

## 🗄️ **Database Schema Updates**

### **New Collections:**
1. **`users`** - User accounts and authentication
2. **`urls`** - Enhanced with user ownership

### **New Indexes:**
- Users: `username` (unique), `email` (unique)
- URLs: `user_id`, `user_id + created_at` (compound)

## 🚀 **Running the Application**

```bash
# Make sure MongoDB is running
# Start the application
go run main.go auth.go handlers.go database.go

# Or build and run
go build -o rapidlink-api
./rapidlink-api
```

The server will start on `http://localhost:8080` with full JWT authentication support!
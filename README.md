# RapidLink API Backend

A high-performance, secure, and scalable URL shortener backend written in Go. This service powers the RapidLink platform, providing RESTful APIs for link shortening, analytics, bulk operations, and user authentication.

## Features
- **Base58 short URLs** for maximum readability
- **Enterprise-grade security**: JWT authentication, AES-256 encryption
- **Real-time analytics** and click tracking
- **Bulk upload** support (CSV)
- **Custom domains** and branded links
- **Self-hosted**: No vendor lock-in
- **Optimized for performance**: 2000+ req/sec

## Requirements
- Go 1.18+
- MongoDB 4.2+
- (Optional) Docker for containerized deployment

## Getting Started

### 1. Clone the repository
```sh
git clone https://github.com/your-org/rapidlink-api.git
cd rapidlink-api
```

### 2. Configure Environment
Create a `.env` file (or set environment variables):
```
BASE_URL=http://localhost:8080
MONGODB_URI=mongodb://localhost:27017/rapidlink
JWT_SECRET=your-very-secret-key
ALLOWED_ORIGINS=*
```

### 3. Run the Server
```sh
go run main.go
```
The server will start on `http://localhost:8080` by default.

### 4. API Endpoints
- `POST   /auth/register` — Register a new user
- `POST   /auth/login` — Login and receive JWT
- `POST   /auth/validate` — Validate JWT
- `GET    /auth/profile` — Get user profile (auth required)
- `PUT    /url` — Shorten a URL (auth required)
- `POST   /bulk` — Bulk upload URLs (auth required)
- `GET    /analytics` — Get analytics (auth required)
- `PUT    /rapidlink-demo` — Demo shortener (no auth)
- `GET    /rapidlink-demo` — Get demo links (no auth)
- `GET    /:short-url` — Redirect to original URL

### 5. Bulk Upload
See [`BULK_UPLOAD_API_SPEC.md`](./BULK_UPLOAD_API_SPEC.md) for CSV format and usage.

## Project Structure
- `main.go` — Entry point, server setup
- `handlers.go` — API handlers
- `database.go` — MongoDB logic
- `security.go` — Security utilities
- `constants.go` — Centralized constants
- `auth.go` — Authentication logic
- `benchmark.go` — Performance tests

## Security
- JWT authentication for all protected endpoints
- AES-256 encryption for sensitive data
- Input sanitization and validation
- Rate limiting and security headers

## License
MIT

---
For more details, see the in-code documentation and additional markdown files in this directory.

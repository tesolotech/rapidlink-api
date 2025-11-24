# 📋 Bulk Upload API Specification

## API Endpoint Overview

**Endpoint**: `POST /bulk`  
**Authentication**: Required (JWT Bearer Token)  
**Content-Type**: `multipart/form-data`  
**Rate Limit**: 1000 URLs per request  
**File Size Limit**: 10MB  

## Request Format

### Headers
```http
Authorization: Bearer <jwt-token>
Content-Type: multipart/form-data
```

### Request Body
```http
POST /bulk HTTP/1.1
Host: localhost:8080
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
Content-Type: multipart/form-data; boundary=----WebKitFormBoundary7MA4YWxkTrZu0gW

------WebKitFormBoundary7MA4YWxkTrZu0gW
Content-Disposition: form-data; name="file"; filename="urls.csv"
Content-Type: text/csv

Long URL,Domain,Custom Alias (optional),Tags,Expires (optional)
https://example.com,http://localhost:8080,,Technology;Education,
https://google.com,http://localhost:8080,google,Search;Tools,2025-12-31
------WebKitFormBoundary7MA4YWxkTrZu0gW--
```

## CSV File Format

### Required Headers
```csv
Long URL,Domain,Custom Alias (optional),Tags,Expires (optional)
```

### Field Specifications

| Field | Type | Required | Description | Example |
|-------|------|----------|-------------|---------|
| Long URL | String | ✅ Yes | Full URL with protocol | `https://example.com` |
| Domain | String | ❌ No | Target domain (uses default if empty) | `http://localhost:8080` |
| Custom Alias | String | ❌ No | Custom short code (must be unique) | `google` |
| Tags | String | ❌ No | Semicolon-separated tags | `Technology;Education` |
| Expires | String | ❌ No | Date in YYYY-MM-DD or RFC3339 format | `2025-12-31` or `2025-12-31T23:59:59Z` |

### Sample CSV Content
```csv
Long URL,Domain,Custom Alias (optional),Tags,Expires (optional)
https://example.com,http://localhost:8080,,Technology;Education,
https://google.com,http://localhost:8080,google,Search;Tools,2025-12-31
https://github.com,http://localhost:8080,github,Development;Code,2026-01-01T23:59:59Z
https://stackoverflow.com,http://localhost:8080,,Programming;Help,
https://mozilla.org,http://localhost:8080,mdn,Documentation;Web,2025-06-30
```

## Response Format

### Success Response (HTTP 200)
```json
{
    "total_processed": 5,
    "successful": 4,
    "failed": 1,
    "processing_time": "2.3s",
    "results": [
        {
            "long_url": "https://example.com",
            "short_url": "abc123",
            "domain": "http://localhost:8080",
            "tags": ["Technology", "Education"],
            "success": true,
            "created_at": "2025-11-21T01:35:03Z"
        },
        {
            "long_url": "https://invalid-url",
            "success": false,
            "error": "Invalid URL format"
        }
    ]
}
```

### Error Responses

#### 400 Bad Request - Invalid File
```json
{
    "error": "Invalid file type. Only CSV files are supported (got: .txt)"
}
```

#### 400 Bad Request - File Too Large
```json
{
    "error": "File too large. Maximum size: 10MB (current: 15.5 MB)"
}
```

#### 401 Unauthorized
```json
{
    "error": "Unauthorized"
}
```

#### 413 Request Entity Too Large
```json
{
    "error": "Too many URLs in file. Maximum allowed: 1000 (found: 1500)"
}
```

#### 500 Internal Server Error
```json
{
    "error": "Failed to process file: database connection timeout"
}
```

## Processing Rules

### URL Validation
- Must include protocol (http:// or https://)
- Must be accessible and valid format
- Cannot be malicious or blocked domains

### Duplicate Handling
- Existing URLs (same URL + domain + user) return existing short code
- Custom aliases must be globally unique
- Duplicate custom aliases result in error for that specific URL

### Tag Processing
- Tags are trimmed and sanitized
- Empty tags are filtered out
- Maximum 10 tags per URL
- Tag names limited to 50 characters each

### Expiration Handling
- Supports YYYY-MM-DD format (sets to end of day)
- Supports full RFC3339 datetime format
- Default expiration: 5 years from creation
- Past dates result in error

## Rate Limiting & Quotas

| Limit Type | Value | Scope |
|------------|-------|-------|
| Max File Size | 10MB | Per request |
| Max URLs per batch | 1000 | Per request |
| Request timeout | 60 seconds | Per request |
| Concurrent workers | 10 | Per request |

## Error Codes & Handling

### Individual URL Errors
- `Invalid URL format` - URL validation failed
- `Custom alias already exists` - Duplicate custom alias
- `Invalid expiration date format` - Date parsing failed
- `Database error` - Database operation failed

### Batch Processing Errors
- Individual failures don't stop batch processing
- Results array contains success/failure status for each URL
- Failed URLs include specific error messages
- Processing continues until all URLs are attempted

## Security Features

### Authentication
- JWT Bearer token required in Authorization header
- Token validation on every request
- User context extracted from token

### Input Validation
- File type validation (CSV only)
- File size limits enforced
- URL format validation
- XSS protection on all inputs

### Audit Logging
- All requests logged with user ID, IP, and timestamp
- Security events tracked (unauthorized access, invalid files, etc.)
- Error conditions logged for monitoring

## Performance Characteristics

### Processing Speed
- **Small batches** (1-100 URLs): ~0.5-2 seconds
- **Medium batches** (100-500 URLs): ~2-8 seconds  
- **Large batches** (500-1000 URLs): ~8-15 seconds

### Throughput Metrics
- **Concurrent processing**: 10 goroutines
- **Database operations**: ~50-200 ops/second
- **Memory usage**: ~5-50MB depending on batch size

### Scalability
- Stateless design supports horizontal scaling
- Database connection pooling optimizes resource usage
- Worker pool pattern prevents resource exhaustion

## Client Implementation Examples

### JavaScript/React
```javascript
const uploadCSV = async (file) => {
    const formData = new FormData();
    formData.append('file', file);
    
    const response = await fetch('/bulk', {
        method: 'POST',
        headers: {
            'Authorization': `Bearer ${token}`
        },
        body: formData
    });
    
    return await response.json();
};
```

### curl Command
```bash
curl -X POST http://localhost:8080/bulk \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -F "file=@/path/to/urls.csv"
```

### Python
```python
import requests

def upload_bulk_urls(file_path, token):
    headers = {'Authorization': f'Bearer {token}'}
    files = {'file': open(file_path, 'rb')}
    
    response = requests.post(
        'http://localhost:8080/bulk',
        headers=headers,
        files=files
    )
    
    return response.json()
```

## Testing & Validation

### Test CSV Templates
Available at: `GET /bulk/template` (planned feature)

### Validation Checklist
- [ ] File format is CSV with proper headers
- [ ] All required fields (Long URL) are present
- [ ] URLs include protocol (http/https)
- [ ] Custom aliases are unique
- [ ] File size under 10MB
- [ ] Less than 1000 URLs per batch

### Error Testing Scenarios
1. **Invalid file format** - Upload .txt file
2. **Missing headers** - CSV without proper column headers
3. **Invalid URLs** - URLs without protocol or malformed
4. **Duplicate aliases** - Same custom alias used multiple times
5. **Large file** - File exceeding 10MB limit
6. **Too many URLs** - More than 1000 URLs in batch

## Monitoring & Observability

### Key Metrics to Track
- Processing time per batch
- Success/failure ratios
- File size distribution
- Error type frequency
- User activity patterns

### Logging Events
- Request start/completion
- Individual URL processing results
- Security violations
- Performance anomalies
- Database connection issues

### Health Checks
- Endpoint availability: `GET /health`
- Database connectivity: `GET /health/db`
- Processing capacity: `GET /health/performance`

---

**API Version**: 1.0.0  
**Last Updated**: November 21, 2025  
**Compatibility**: Go 1.21+, MongoDB 4.4+
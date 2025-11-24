# MongoDB Schema Design for URL Shortener

## Collection: `urls`

### Document Structure

```json
{
  "_id": ObjectId("..."),
  "short_url": "abc123",
  "long_url": "https://example.com/very/long/url",
  "created_at": ISODate("2025-11-16T10:30:00.000Z"),
  "expires_at": ISODate("2025-12-16T10:30:00.000Z"), // null if no expiry
  "clicks": 0,
  "is_active": true,
  "last_clicked": ISODate("2025-11-16T12:15:00.000Z"), // null if never clicked
  "click_history": [
    {
      "timestamp": ISODate("2025-11-16T12:15:00.000Z"),
      "ip": "192.168.1.1",
      "user_agent": "Mozilla/5.0..."
    }
  ]
}
```

### Required Indexes

1. **Primary Index**: `short_url` (unique)
   - Purpose: Fast lookup during redirects
   - Type: Unique index
   ```javascript
   db.urls.createIndex({ "short_url": 1 }, { unique: true })
   ```

2. **1-to-1 Mapping Index**: `long_url` (unique for active URLs)
   - Purpose: Ensure same long URL gets same short URL
   - Type: Partial unique index (only for active URLs)
   ```javascript
   db.urls.createIndex(
     { "long_url": 1 },
     {
       unique: true,
       partialFilterExpression: { "is_active": true }
     }
   )
   ```

3. **Expiry Index**: `expires_at`
   - Purpose: Efficient cleanup of expired URLs
   - Type: Sparse index with TTL
   ```javascript
   db.urls.createIndex({ "expires_at": 1 }, { sparse: true })
   ```

4. **Analytics Index**: `created_at`
   - Purpose: Time-based analytics queries
   ```javascript
   db.urls.createIndex({ "created_at": -1 })
   ```

5. **Compound Index**: `is_active + created_at`
   - Purpose: Efficiently query active URLs by creation time
   ```javascript
   db.urls.createIndex({ "is_active": 1, "created_at": -1 })
   ```

### Schema Validation

```javascript
db.createCollection("urls", {
  validator: {
    $jsonSchema: {
      bsonType: "object",
      required: ["short_url", "long_url", "created_at", "is_active"],
      properties: {
        short_url: {
          bsonType: "string",
          minLength: 1,
          maxLength: 50,
          description: "Short URL identifier - required and unique"
        },
        long_url: {
          bsonType: "string",
          minLength: 1,
          maxLength: 2048,
          description: "Original long URL - required"
        },
        created_at: {
          bsonType: "date",
          description: "Creation timestamp - required"
        },
        expires_at: {
          bsonType: ["date", "null"],
          description: "Expiration timestamp - optional"
        },
        clicks: {
          bsonType: "int",
          minimum: 0,
          description: "Number of times URL was accessed"
        },
        is_active: {
          bsonType: "bool",
          description: "Whether URL is currently active"
        },
        last_clicked: {
          bsonType: ["date", "null"],
          description: "Last access timestamp"
        },
        click_history: {
          bsonType: "array",
          items: {
            bsonType: "object",
            properties: {
              timestamp: { bsonType: "date" },
              ip: { bsonType: "string" },
              user_agent: { bsonType: "string" }
            }
          },
          description: "Detailed click analytics"
        }
      }
    }
  }
})
```

## Benefits of This Schema Design

1. **1-to-1 Mapping**: Partial unique index on `long_url` ensures same long URL gets same short URL
2. **Performance**: Optimized indexes for all query patterns (lookup, analytics, cleanup)
3. **Analytics**: Built-in click tracking with detailed history
4. **Expiry Management**: Proper TTL and expiry handling
5. **Scalability**: Designed for high-performance operations
6. **Data Integrity**: Schema validation ensures data consistency
7. **Flexibility**: Support for tags, custom URLs, and extensible click tracking

## Query Patterns Supported

- Fast redirect lookup: `db.urls.findOne({short_url: "abc123", is_active: true})`
- Check existing long URL: `db.urls.findOne({long_url: "...", is_active: true})`
- Analytics by time range: `db.urls.find({created_at: {$gte: start, $lte: end}})`
- Cleanup expired URLs: `db.urls.updateMany({expires_at: {$lte: new Date()}}, {$set: {is_active: false}})`
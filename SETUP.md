# MongoDB Setup for URL Shortener

## Installation & Setup

### 1. Install MongoDB

**Windows (using Chocolatey):**
```bash
choco install mongodb
```

**Or download from:** https://www.mongodb.com/try/download/community

### 2. Start MongoDB Service

**Windows:**
```bash
# Start MongoDB service
net start MongoDB

# Or run manually
mongod --dbpath C:\data\db
```

**Linux/Mac:**
```bash
sudo systemctl start mongod
# or
brew services start mongodb-community
```

### 3. Initialize Database and Indexes

Run this script in MongoDB shell (`mongosh`):

```javascript
// Connect to the database
use url_shortener

// Create the collection with validation
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
        custom: {
          bsonType: "bool",
          description: "Whether this is a custom short URL"
        },
        tags: {
          bsonType: "array",
          items: {
            bsonType: "string"
          },
          description: "Array of tags for categorization"
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

// Create indexes (will be auto-created by the Go application)
// But you can create them manually if needed:

// 1. Unique index on short_url
db.urls.createIndex({ "short_url": 1 }, { unique: true })

// 2. Partial unique index on long_url (only for active URLs)
db.urls.createIndex(
  { "long_url": 1 }, 
  { 
    unique: true,
    partialFilterExpression: { "is_active": true }
  }
)

// 3. Index on expires_at for cleanup operations
db.urls.createIndex({ "expires_at": 1 }, { sparse: true })

// 4. Index on created_at for analytics
db.urls.createIndex({ "created_at": -1 })

// 5. Compound index on is_active and created_at
db.urls.createIndex({ "is_active": 1, "created_at": -1 })

print("Database setup completed!")
```

### 4. Environment Configuration

You can customize the MongoDB connection by modifying these values in `main.go`:

```go
connectionString := "mongodb://localhost:27017" // Change as needed
databaseName := "url_shortener"                 // Change as needed
```

For production, consider using environment variables:
```go
connectionString := os.Getenv("MONGODB_URI") // e.g., "mongodb://user:pass@host:port/db"
if connectionString == "" {
    connectionString = "mongodb://localhost:27017"
}
```

### 5. Running the Application

```bash
# Build the application
go build -o rapidlink-api.exe

# Run the application
./rapidlink-api.exe
```

The application will:
- Connect to MongoDB on startup
- Create indexes automatically
- Run a background cleanup job every hour for expired URLs
- Listen on port 8080

### 6. Testing the Setup

Test with curl commands:

```bash
# Create a short URL
curl -X PUT http://localhost:8080/url \
  -H "Content-Type: application/json" \
  -d '{"long-url": "https://example.com", "tags": ["test"]}'

# Access the short URL (replace abc123 with actual short URL)
curl -I http://localhost:8080/abc123

# Get analytics
curl "http://localhost:8080/analytics?short_url=abc123"
```

### 7. MongoDB Monitoring

Check the database:
```bash
# Connect to MongoDB
mongosh

# Use the database
use url_shortener

# Check collections
show collections

# Check documents
db.urls.find().pretty()

# Check indexes
db.urls.getIndexes()

# Collection stats
db.urls.stats()
```
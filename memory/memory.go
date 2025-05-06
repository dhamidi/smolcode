package memory

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// MemoryManager manages memories stored in a SQLite database.
type MemoryManager struct {
	db *sql.DB
}

// Memory represents a single piece of information.
type Memory struct {
	ID      string
	Content string
}

// New creates a new MemoryManager instance and initializes the database.
// dbPath is the path to the SQLite database file.
func New(dbPath string) (*MemoryManager, error) {
	// Ensure the directory for the database file exists.
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory %s: %w", dbDir, err)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database at %s: %w", dbPath, err)
	}

	// Create memories table if it doesn't exist.
	// Using TEXT for ID as it's a string, and TEXT for Content.
	// ROWID is automatically an alias for the primary key in SQLite if not specified otherwise.
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS memories (
		id TEXT PRIMARY KEY,
		content TEXT NOT NULL
	);
	`
	_, err = db.Exec(createTableSQL)
	if err != nil {
		db.Close() // Close the DB if table creation fails
		return nil, fmt.Errorf("failed to create memories table: %w", err)
	}

	// Create FTS5 virtual table for full-text search if it doesn't exist.
	// It will use the 'content' column from the 'memories' table.
	// We use 'content=' to specify which column of the external content table to use.
	// Using 'id' as the column name for the document ID in the FTS table, mapping to 'id' from 'memories'.
	createFTSTableSQL := `
	CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
		id, -- Corresponds to the id column in the memories table
		content, -- The column to be indexed for full-text search
		content='memories', -- Specifies the external content table
		content_rowid='id' -- Specifies which column in 'memories' acts as the rowid for FTS updates (must be PRIMARY KEY)
	);
	`
	// Note: For fts5 with external content, the content_rowid must map to the *actual* rowid or an INTEGER PRIMARY KEY of the external table.
	// Since our `id` is TEXT PRIMARY KEY, this setup is more complex. A common pattern is to have an explicit INTEGER PRIMARY KEY in the main table.
	// Let's adjust 'memories' to have an explicit integer primary key for FTS linkage, and 'id' as a unique text identifier.

	// Revised schema for 'memories' for better FTS integration:
	// 1. Drop existing tables if they were made with the old schema (for dev, not prod)
	// For simplicity here, we'll assume a fresh setup or handle migration elsewhere.
	// We should ensure this works by potentially dropping and recreating if schema is wrong, or versioning.

	// Let's re-do table creation with a proper integer primary key for FTS
	// We need to ensure this runs only if the schema is not as expected or on first run.
	// For now, just create if not exists with the new schema.

	initSQL := `
	CREATE TABLE IF NOT EXISTS memories (
		docid INTEGER PRIMARY KEY AUTOINCREMENT, -- Integer primary key for FTS
		id TEXT UNIQUE NOT NULL, -- User-facing string ID
		content TEXT NOT NULL
	);

	CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
		content,             -- Column to be indexed from 'memories'
		content='memories',    -- External content table is 'memories'
		content_rowid='docid'  -- Link to 'docid' INTEGER PRIMARY KEY of 'memories'
	);

	-- Triggers to keep FTS table synchronized with the memories table
	CREATE TRIGGER IF NOT EXISTS memories_ai AFTER INSERT ON memories BEGIN
		INSERT INTO memories_fts (rowid, content) VALUES (new.docid, new.content);
	END;
	CREATE TRIGGER IF NOT EXISTS memories_ad AFTER DELETE ON memories BEGIN
		INSERT INTO memories_fts (memories_fts, rowid, content) VALUES ('delete', old.docid, old.content);
	END;
	CREATE TRIGGER IF NOT EXISTS memories_au AFTER UPDATE ON memories BEGIN
		INSERT INTO memories_fts (memories_fts, rowid, content) VALUES ('delete', old.docid, old.content);
		INSERT INTO memories_fts (rowid, content) VALUES (new.docid, new.content);
	END;
	`

	_, err = db.Exec(initSQL)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize database schema: %w", err)
	}

	return &MemoryManager{
		db: db,
	}, nil
}

// Close closes the database connection.
func (m *MemoryManager) Close() error {
	if m.db != nil {
		return m.db.Close()
	}
	return nil // Or return an error if db was unexpectedly nil?
}

// AddMemory adds a new memory or replaces an existing one with the same ID.
func (m *MemoryManager) AddMemory(id string, content string) error {
	// Use INSERT OR REPLACE based on the unique constraint on the 'id' column.
	// The triggers will handle updating the FTS table.
	insertSQL := `
	INSERT INTO memories (id, content)
	VALUES (?, ?)
	ON CONFLICT(id) DO UPDATE SET
		content = excluded.content;
	`

	_, err := m.db.Exec(insertSQL, id, content)
	if err != nil {
		return fmt.Errorf("failed to insert/replace memory with id %s: %w", id, err)
	}

	return nil
}

// GetMemoryByID retrieves a specific memory by its user-defined ID.
func (m *MemoryManager) GetMemoryByID(id string) (*Memory, error) {
	querySQL := `SELECT id, content FROM memories WHERE id = ?;`
	row := m.db.QueryRow(querySQL, id)

	mem := &Memory{}
	err := row.Scan(&mem.ID, &mem.Content)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("memory with id '%s' not found: %w", id, err) // Consider a custom error type or just sql.ErrNoRows
		}
		return nil, fmt.Errorf("failed to retrieve memory with id '%s': %w", id, err)
	}
	return mem, nil
}

// SearchMemory performs a full-text search on the content of memories.
// It returns a slice of matching Memory structs.
func (m *MemoryManager) SearchMemory(query string) ([]*Memory, error) {
	// Query the FTS table and join with the memories table to get the original id and content.
	// The FTS table (memories_fts) stores rowid which corresponds to docid in the memories table.
	searchSQL := `
	SELECT m.id, m.content
	FROM memories AS m
	JOIN memories_fts AS fts ON m.docid = fts.rowid
	WHERE fts.memories_fts MATCH ?
	ORDER BY rank; -- rank is an auxiliary function of FTS5, orders by relevance
	`

	rows, err := m.db.Query(searchSQL, query)
	if err != nil {
		return nil, fmt.Errorf("failed to execute search query '%s': %w", query, err)
	}
	defer rows.Close()

	var memories []*Memory
	for rows.Next() {
		mem := &Memory{}
		if err := rows.Scan(&mem.ID, &mem.Content); err != nil {
			return nil, fmt.Errorf("failed to scan search result: %w", err)
		}
		memories = append(memories, mem)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating search results: %w", err)
	}

	return memories, nil
}

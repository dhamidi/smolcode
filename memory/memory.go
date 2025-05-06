package memory

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

var ErrNotFound = errors.New("memory: not found")

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

	if err := initializeSchema(db); err != nil {
		db.Close() // Ensure db is closed if schema initialization fails
		// The error from initializeSchema is already specific, so we can return it directly or wrap it.
		// Wrapping it provides context that initialization failed within New().
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	return &MemoryManager{
		db: db,
	}, nil
}

func initializeSchema(db *sql.DB) error {
	// Create memories table if it doesn't exist.
	// Using TEXT for ID as it's a string, and TEXT for Content.
	// ROWID is automatically an alias for the primary key in SQLite if not specified otherwise.

	// The main table creation, FTS table, and triggers are handled by initSQL below.

	// Create FTS5 virtual table for full-text search if it doesn't exist.
	// It will use the 'content' column from the 'memories' table.
	// We use 'content=' to specify which column of the external content table to use.
	// Using 'id' as the column name for the document ID in the FTS table, mapping to 'id' from 'memories'.
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

	_, err := db.Exec(initSQL)
	if err != nil {
		// No need to db.Close() here as the caller (New) will do it.
		return fmt.Errorf("failed to execute schema initialization SQL: %w", err)
	}
	return nil
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
		if errors.Is(err, sql.ErrNoRows) { // Check if the underlying error is sql.ErrNoRows
			return nil, fmt.Errorf("memory with id '%s': %w", id, ErrNotFound)
		}
		return nil, fmt.Errorf("failed to retrieve memory with id '%s': %w", id, err)
	}
	return mem, nil
}

// SearchMemory performs a full-text search on the content of memories.
// It returns a slice of matching Memory structs.
func (m *MemoryManager) SearchMemory(query string) ([]*Memory, error) {
	// Step 1: Get matching docids from FTS table
	ftsQuerySQL := `
	SELECT fts.rowid -- This is the docid
	FROM memories_fts AS fts
	WHERE fts.memories_fts MATCH ?
	ORDER BY rank;
	`
	rows, err := m.db.Query(ftsQuerySQL, query)
	if err != nil {
		return nil, fmt.Errorf("failed to execute FTS query for docids: %w", err)
	}
	// It's important to close rows from the first query before the loop for the second query starts,
	// especially if not all rows are consumed by Next(). However, Scan consumes them.
	// defer rows.Close() // Standard defer is fine if loop completes or errors out.

	var docIDs []int64
	for rows.Next() {
		var docID int64
		if err := rows.Scan(&docID); err != nil {
			rows.Close() // Close explicitly on error before returning
			return nil, fmt.Errorf("failed to scan docID from FTS results: %w", err)
		}
		docIDs = append(docIDs, docID)
	}
	if err = rows.Err(); err != nil {
		rows.Close() // Close explicitly on error
		return nil, fmt.Errorf("error iterating FTS docID results: %w", err)
	}
	rows.Close() // Ensure rows are closed after successful iteration

	if len(docIDs) == 0 {
		return []*Memory{}, nil // No matches
	}

	// Step 2: Retrieve full memory objects for each docID
	var memories []*Memory

	stmtSQL := `SELECT id, content FROM memories WHERE docid = ?;`
	stmt, err := m.db.Prepare(stmtSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare statement to get memory by docid: %w", err)
	}
	defer stmt.Close()

	for _, docID := range docIDs {
		mem := &Memory{}
		err := stmt.QueryRow(docID).Scan(&mem.ID, &mem.Content)
		if err != nil {
			if err == sql.ErrNoRows {
				// This would be strange if FTS returned a docID that doesn't exist, implies data inconsistency
				return nil, fmt.Errorf("FTS returned docID %d but no matching memory found (data inconsistency?): %w", docID, err)
			}
			return nil, fmt.Errorf("failed to retrieve memory for docID %d: %w", docID, err)
		}
		memories = append(memories, mem)
	}

	return memories, nil
}

// Forget removes a memory by its user-defined ID.
func (m *MemoryManager) Forget(id string) error {
	deleteSQL := `DELETE FROM memories WHERE id = ?;`
	result, err := m.db.Exec(deleteSQL, id)
	if err != nil {
		return fmt.Errorf("failed to delete memory with id %s: %w", id, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		// This error is less critical but good to log if it happens.
		// We won't fail the operation just because we can't get RowsAffected.
		fmt.Printf("Warning: could not get rows affected after deleting memory %s: %v\n", id, err)
	} else if rowsAffected == 0 {
		return fmt.Errorf("memory with id '%s' not found to forget: %w", id, ErrNotFound) // Now using custom ErrNotFound
	}

	return nil
}

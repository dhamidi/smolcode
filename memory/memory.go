package memory

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

const (
	dbDir      = ".smolcode/data"
	dbFileName = "memory.db"
)

var dbPath = filepath.Join(dbDir, dbFileName)

// Fact represents a piece of information stored in the memory.
type Fact struct {
	ID      string
	Content string
}

// InitDB initializes the SQLite database.
// It creates the necessary directory, database file, and tables if they don't exist.
func InitDB() (*sql.DB, error) {
	// Ensure the database directory exists
	err := os.MkdirAll(dbDir, 0755)
	if err != nil {
		return nil, fmt.Errorf("failed to create database directory %s: %w", dbDir, err)
	}

	// Open the database, creating it if it doesn't exist
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database at %s: %w", dbPath, err)
	}

	// Create the main facts table
	createFactsTableSQL := `
CREATE TABLE IF NOT EXISTS facts (
    id TEXT PRIMARY KEY,
    content TEXT NOT NULL
);`
	_, err = db.Exec(createFactsTableSQL)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create facts table: %w", err)
	}

	ftsTableSQL := `CREATE VIRTUAL TABLE IF NOT EXISTS facts_fts USING fts5(
    content,
    content_table='facts',
    content_rowid='id'
);`
	_, err = db.Exec(ftsTableSQL)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create facts_fts virtual table: %w", err)
	}

	triggerInsertSQL := `CREATE TRIGGER IF NOT EXISTS facts_after_insert
    AFTER INSERT ON facts
    BEGIN
        INSERT INTO facts_fts (rowid, content) VALUES (NEW.id, NEW.content);
    END;`
	_, err = db.Exec(triggerInsertSQL)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create facts_after_insert trigger: %w", err)
	}

	triggerDeleteSQL := `CREATE TRIGGER IF NOT EXISTS facts_after_delete
    AFTER DELETE ON facts
    BEGIN
        DELETE FROM facts_fts WHERE rowid = OLD.id;
    END;`
	_, err = db.Exec(triggerDeleteSQL)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create facts_after_delete trigger: %w", err)
	}

	triggerUpdateSQL := `CREATE TRIGGER IF NOT EXISTS facts_after_update
    AFTER UPDATE ON facts
    BEGIN
        UPDATE facts_fts SET content = NEW.content WHERE rowid = OLD.id;
    END;`
	_, err = db.Exec(triggerUpdateSQL)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create facts_after_update trigger: %w", err)
	}

	return db, nil
}

// AddFact adds a new fact or replaces an existing one in the database.
// The associated FTS index is updated via triggers.
func AddFact(db *sql.DB, id string, content string) error {
	if db == nil {
		return fmt.Errorf("database connection is nil")
	}
	if id == "" {
		return fmt.Errorf("fact ID cannot be empty")
	}

	stmt, err := db.Prepare("INSERT OR REPLACE INTO facts (id, content) VALUES (?, ?)")
	if err != nil {
		return fmt.Errorf("failed to prepare statement for AddFact: %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(id, content)
	if err != nil {
		return fmt.Errorf("failed to execute AddFact statement for id '%s': %w", id, err)
	}

	return nil
}

// SearchFacts searches for facts in the database using FTS5.
// It returns a slice of matching Fact structs.
func SearchFacts(db *sql.DB, query string) ([]Fact, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}
	if query == "" {
		return []Fact{}, nil // Return empty slice for empty query, not an error
	}

	// The query will search against the 'content' column in 'facts_fts'.
	// We select the rowid (which is the original 'id' from the 'facts' table due to content_rowid='id')
	// and the actual content. Since 'facts_fts' is an external content table using 'facts',
	// we can directly query 'facts_fts' and get the necessary data or join back to 'facts' if needed.
	// For FTS5 with external content, rowid of the FTS table is the rowid of the content table.
	// We also need the original content, which FTS5 can provide using the `content` column if not unindexed,
	// or we can JOIN with the original `facts` table using the rowid.
	// `SELECT f.id, f.content FROM facts f JOIN facts_fts fts ON f.id = fts.rowid WHERE fts.facts_fts MATCH ? ORDER BY rank`
	// The `rank` is an auxiliary function from FTS that indicates relevance.
	// SQLite FTS5 documentation: "The FTS table column named 'rowid' is equivalent to the rowid of the external content table."
	// So, fts.rowid is facts.id in our case.

	searchSQL := `
SELECT
    f.id,      -- The original ID from the facts table
    f.content  -- The original content from the facts table
FROM facts_fts AS fts
JOIN facts AS f ON fts.rowid = f.id
WHERE fts.facts_fts MATCH ? ORDER BY rank; -- rank is implicitly available
`
	// Note: The table name `facts_fts` is used in `MATCH`. FTS5 then knows which column to search (the one not marked UNINDEXED).
	// If we had multiple indexed columns in facts_fts, we could specify: `WHERE facts_fts.content MATCH ?`
	// But since `content` is the only indexed column (implicitly, as it's not `UNINDEXED`), `facts_fts MATCH ?` is fine.

	stmt, err := db.Prepare(searchSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare search statement: %w", err)
	}
	defer stmt.Close()

	rows, err := stmt.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to execute search query '%s': %w", query, err)
	}
	defer rows.Close()

	var results []Fact
	for rows.Next() {
		var fact Fact
		if err := rows.Scan(&fact.ID, &fact.Content); err != nil {
			return nil, fmt.Errorf("failed to scan search result row: %w", err)
		}
		results = append(results, fact)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error encountered during row iteration: %w", err)
	}

	return results, nil
}

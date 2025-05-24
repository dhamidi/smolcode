package history

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// GetLatestConversationID retrieves the ID of the most recent conversation
// from the database at the given dbPath.
// It returns the conversation ID and nil on success.
// If no conversations are found, it returns an empty string and sql.ErrNoRows.
// Other errors from database interaction are returned as well.
func GetLatestConversationID(dbPath string) (string, error) {
	db, err := initDB(dbPath) // initDB is defined in history.go
	if err != nil {
		return "", fmt.Errorf("failed to open/initialize database at %s: %w", dbPath, err)
	}
	defer db.Close()

	var id string
	query := "SELECT id FROM conversations ORDER BY created_at DESC LIMIT 1"
	err = db.QueryRow(query).Scan(&id)
	if err != nil {
		if err == sql.ErrNoRows {
			// It's idiomatic to return sql.ErrNoRows if no record is found
			return "", err
		}
		return "", fmt.Errorf("failed to query for latest conversation ID: %w", err)
	}
	return id, nil
}

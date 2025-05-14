package history

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// ListConversations retrieves metadata for all stored conversations from the specified SQLite database.
func ListConversations(dbPath string) ([]ConversationMetadata, error) {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("database file does not exist: %s", dbPath)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	query := `
		SELECT
			c.id,
			c.created_at,
			COUNT(m.id) as message_count,
			COALESCE(MAX(m.created_at), c.created_at) as latest_message_at 
		FROM
			conversations c
		LEFT JOIN
			messages m ON c.id = m.conversation_id
		GROUP BY
			c.id
		ORDER BY
			latest_message_at DESC;
	`

	rows, err := db.Query(query)
	if err != nil {
		// Check if the error is due to missing tables (e.g., new/empty but valid DB file)
		var tableCheck string
		errTable := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='conversations'").Scan(&tableCheck)
		if errTable == sql.ErrNoRows {
			return []ConversationMetadata{}, nil // No 'conversations' table, so no conversations
		}
		return nil, fmt.Errorf("failed to query conversations: %w", err)
	}
	defer rows.Close()

	var metadataList []ConversationMetadata
	for rows.Next() {
		var meta ConversationMetadata
		var createdAtStr string
		var latestTimestampStr string

		if err := rows.Scan(&meta.ID, &createdAtStr, &meta.MessageCount, &latestTimestampStr); err != nil {
			return nil, fmt.Errorf("failed to scan conversation metadata: %w", err)
		}

		meta.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAtStr)
		if err != nil {
			meta.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr) // Fallback
			if err != nil {
				return nil, fmt.Errorf("failed to parse conversation created_at '%s': %w", createdAtStr, err)
			}
		}

		// Parse the latest_message_at string into time.Time
		meta.LatestMessageTime, err = time.Parse(time.RFC3339Nano, latestTimestampStr)
		if err != nil {
			// Attempt to parse with a different common SQLite format if the first one fails
			meta.LatestMessageTime, err = time.Parse("2006-01-02 15:04:05.999999999-07:00", latestTimestampStr)
			if err != nil {
				// Fallback to RFC3339 if the specific sqlite one also fails
				meta.LatestMessageTime, err = time.Parse(time.RFC3339, latestTimestampStr)
				if err != nil {
					return nil, fmt.Errorf("failed to parse latest_message_timestamp '%s': %w", latestTimestampStr, err)
				}
			}
		}
		metadataList = append(metadataList, meta)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return metadataList, nil
}

package history

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// LoadFrom retrieves a specific conversation and its messages from the database at dbPath.
func LoadFrom(conversationID string, dbPath string) (*Conversation, error) {
	db, err := initDB(dbPath) // initDB also opens the connection
	if err != nil {
		// If initDB failed because the path doesn't exist (e.g. os.IsNotExist(err) from os.Stat within initDB)
		// or if sql.Open failed, we return an error indicating DB access issues.
		return nil, fmt.Errorf("failed to open/initialize database at %s: %w", dbPath, err)
	}
	defer db.Close()

	conv := &Conversation{ID: conversationID, Messages: make([]*Message, 0)}

	// Load conversation metadata (specifically CreatedAt)
	err = db.QueryRow("SELECT created_at FROM conversations WHERE id = ?", conversationID).Scan(&conv.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("conversation with ID '%s' not found: %w", conversationID, err) // Consider a custom error type e.g. ErrConversationNotFound
		}
		return nil, fmt.Errorf("failed to query conversation metadata for ID '%s': %w", conversationID, err)
	}

	// Load messages for the conversation
	rows, err := db.Query("SELECT sequence_number, payload, created_at FROM messages WHERE conversation_id = ? ORDER BY sequence_number ASC", conversationID)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages for conversation ID '%s': %w", conversationID, err)
	}
	defer rows.Close()

	type indexedMessage struct {
		seq     int
		message *Message
	}
	var tempMessages []indexedMessage

	for rows.Next() {
		var seq int
		var payloadJSON []byte
		var createdAt time.Time
		msg := &Message{}

		if err := rows.Scan(&seq, &payloadJSON, &createdAt); err != nil {
			return nil, fmt.Errorf("failed to scan message for conversation ID '%s': %w", conversationID, err)
		}

		if err := json.Unmarshal(payloadJSON, &msg.Payload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal message payload for conversation ID '%s': %w", conversationID, err)
		}
		msg.CreatedAt = createdAt
		tempMessages = append(tempMessages, indexedMessage{seq: seq, message: msg})
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error during message rows iteration for conversation ID '%s': %w", conversationID, err)
	}

	// Sort by sequence number just in case (though ORDER BY should handle it)
	sort.SliceStable(tempMessages, func(i, j int) bool {
		return tempMessages[i].seq < tempMessages[j].seq
	})

	for _, im := range tempMessages {
		conv.Messages = append(conv.Messages, im.message)
	}

	return conv, nil
}

// Load retrieves a specific conversation and its messages from the DefaultDatabasePath.
func Load(conversationID string) (*Conversation, error) {
	return LoadFrom(conversationID, DefaultDatabasePath)
}

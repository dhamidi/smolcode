package history

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

//go:embed schema.sql
var schemaSQL string

// DefaultDatabasePath is the default path where the history database is stored.
var DefaultDatabasePath = ".smolcode/history.db"

// Message represents a single message within a conversation, including its content and timestamp.
type Message struct {
	Payload   interface{}
	CreatedAt time.Time
}

// Conversation stores a conversation's ID and its messages.
type Conversation struct {
	ID        string
	Messages  []*Message
	CreatedAt time.Time
}

// New creates a new Conversation with a unique ID and an empty list of messages.
func New() (*Conversation, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}
	return &Conversation{
		ID:        id.String(),
		Messages:  make([]*Message, 0),
		CreatedAt: time.Now(),
	}, nil
}

// Append adds a given message to the end of the in-memory message list for the Conversation.
// The message is wrapped in a Message struct, which includes a timestamp.
func (c *Conversation) Append(payload interface{}) {
	msg := &Message{
		Payload:   payload,
		CreatedAt: time.Now(),
	}
	c.Messages = append(c.Messages, msg)
}

// initializeSchema creates the database schema if it doesn't exist.
func initializeSchema(db *sql.DB) error {
	_, err := db.Exec(schemaSQL)
	return err
}

// initDB ensures the database and tables exist, returning a connection.
func initDB(dataSourceName string) (*sql.DB, error) {
	dbDir := filepath.Dir(dataSourceName)
	if _, err := os.Stat(dbDir); os.IsNotExist(err) {
		err = os.MkdirAll(dbDir, 0755)
		if err != nil {
			return nil, err
		}
	}

	db, err := sql.Open("sqlite3", dataSourceName)
	if err != nil {
		return nil, err
	}

	if err := initializeSchema(db); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

// SaveTo persists the conversation to the database at the specified dbPath.
// It saves the conversation ID and all its messages.
// If messages for this conversation ID already exist, they are cleared and replaced with the current messages.
func SaveTo(conversation *Conversation, dbPath string) error {
	db, err := initDB(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return err
	}

	_, err = tx.Exec(`INSERT OR IGNORE INTO conversations (id, created_at) VALUES (?, ?);`, conversation.ID, conversation.CreatedAt)
	if err != nil {
		tx.Rollback()
		return err
	}

	_, err = tx.Exec(`DELETE FROM messages WHERE conversation_id = ?;`, conversation.ID)
	if err != nil {
		tx.Rollback()
		return err
	}

	stmt, err := tx.Prepare(`INSERT INTO messages (conversation_id, sequence_number, payload, created_at) VALUES (?, ?, ?, ?);`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for i, msg := range conversation.Messages {
		payload, jsonErr := json.Marshal(msg.Payload) // Marshal only the payload
		if jsonErr != nil {
			tx.Rollback()
			return jsonErr
		}
		_, err = stmt.Exec(conversation.ID, i, payload, msg.CreatedAt)
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

// Save persists the conversation to the database using the DefaultDatabasePath.
// It saves the conversation ID and all its messages.
// If messages for this conversation ID already exist, they are cleared and replaced with the current messages.
func Save(conversation *Conversation) error {
	return SaveTo(conversation, DefaultDatabasePath)
}

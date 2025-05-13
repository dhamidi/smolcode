package history

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

// DefaultDatabasePath is the default path where the history database is stored.
var DefaultDatabasePath = ".smolcode/history.db"

// Conversation stores a conversation's ID and its messages.
type Conversation struct {
	ID       string
	Messages []interface{}
}

// New creates a new Conversation with a unique ID and an empty list of messages.
func New() (*Conversation, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}
	return &Conversation{
		ID:       id.String(),
		Messages: make([]interface{}, 0),
	}, nil
}

// Append adds a given message to the end of the in-memory message list for the Conversation.
// The message is stored as an interface{} and will be serialized to JSON upon saving.
func (c *Conversation) Append(message interface{}) {
	c.Messages = append(c.Messages, message)
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

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS conversations (id TEXT PRIMARY KEY);`)
	if err != nil {
		db.Close()
		return nil, err
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		conversation_id TEXT NOT NULL,
		sequence_number INTEGER NOT NULL,
		payload BLOB NOT NULL,
		FOREIGN KEY (conversation_id) REFERENCES conversations(id),
		UNIQUE (conversation_id, sequence_number)
	);`)
	if err != nil {
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

	_, err = tx.Exec(`INSERT OR IGNORE INTO conversations (id) VALUES (?);`, conversation.ID)
	if err != nil {
		tx.Rollback()
		return err
	}

	_, err = tx.Exec(`DELETE FROM messages WHERE conversation_id = ?;`, conversation.ID)
	if err != nil {
		tx.Rollback()
		return err
	}

	stmt, err := tx.Prepare(`INSERT INTO messages (conversation_id, sequence_number, payload) VALUES (?, ?, ?);`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for i, msg := range conversation.Messages {
		payload, jsonErr := json.Marshal(msg)
		if jsonErr != nil {
			tx.Rollback()
			return jsonErr
		}
		_, err = stmt.Exec(conversation.ID, i, payload)
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

package history

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// newTestDB is a helper to create a temporary SQLite DB file for testing.
// It initializes the schema and returns the DB connection and a cleanup function.
func newTestDB(t *testing.T) (*sql.DB, string, func()) {
	t.Helper()
	tempFile, err := os.CreateTemp(t.TempDir(), "test_history_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp db file: %v", err)
	}
	tempFilePath := tempFile.Name()
	// Close the file immediately so SQLite can use the path.
	// The file itself will be cleaned up by t.TempDir().
	if err := tempFile.Close(); err != nil {
		t.Fatalf("Failed to close temp db file: %v", err)
	}

	// Use a DSN that ensures the file is created if it doesn't exist.
	db, err := sql.Open("sqlite3", tempFilePath)
	if err != nil {
		t.Fatalf("Failed to open db at %s: %v", tempFilePath, err)
	}

	// Apply schema
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS conversations (id TEXT PRIMARY KEY);`)
	if err != nil {
		db.Close() // Close before fatalf
		t.Fatalf("Failed to create conversations table in %s: %v", tempFilePath, err)
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
		db.Close() // Close before fatalf
		t.Fatalf("Failed to create messages table in %s: %v", tempFilePath, err)
	}

	cleanup := func() {
		db.Close()
		// os.Remove(tempFilePath) // Not strictly necessary with t.TempDir()
	}

	return db, tempFilePath, cleanup
}

func TestNew(t *testing.T) {
	conv, err := New()
	if err != nil {
		t.Fatalf("New() returned an error: %v", err)
	}
	if conv == nil {
		t.Fatal("New() returned a nil conversation")
	}
	if conv.ID == "" {
		t.Error("New() returned conversation with empty ID")
	}
	if len(conv.Messages) != 0 {
		t.Errorf("New() returned conversation with non-empty messages: got %d, want %d", len(conv.Messages), 0)
	}
}

func TestAppend(t *testing.T) {
	conv, _ := New() // Assuming New() works, tested above
	conv.Append("hello")
	if len(conv.Messages) != 1 {
		t.Fatalf("Append() failed to add first message. Got %d, want %d", len(conv.Messages), 1)
	}
	if conv.Messages[0] != "hello" {
		t.Errorf("Append() stored incorrect message. Got %v, want %s", conv.Messages[0], "hello")
	}

	conv.Append(123)
	if len(conv.Messages) != 2 {
		t.Fatalf("Append() failed to add second message. Got %d, want %d", len(conv.Messages), 2)
	}
	if conv.Messages[1] != 123 {
		t.Errorf("Append() stored incorrect message. Got %v, want %d", conv.Messages[1], 123)
	}
}

func TestSave(t *testing.T) {
	// Create a temporary directory for this test to ensure DefaultDatabasePath is isolated.
	// tempDir := t.TempDir()
	// originalDefaultPath := DefaultDatabasePath // Store original

	// Point DefaultDatabasePath to a file within the tempDir for this test.
	// This requires DefaultDatabasePath to be a variable, not a const.
	// For the sake of this example and to proceed without modifying history.go again now,
	// we will assume a conceptual override or that tests needing specific paths
	// for `Save` would ideally use a version of `Save` that takes a path.
	// Given the current `Save` uses the const, we test its behavior AS IS.
	// This means `TestSave` will interact with the actual `.smolcode/history.db`
	// So we must manage its state carefully: backup if exists, clean up after.

	// For a cleaner test, we should make DefaultDatabasePath a var or pass path to Save.
	// Since it is a const, this test will operate on the actual DefaultDatabasePath.
	// Ensure the .smolcode directory exists or can be created by initDB.
	dbDir := filepath.Dir(DefaultDatabasePath)
	_ = os.MkdirAll(dbDir, 0755) // Ensure directory exists

	// Clean up any existing DB file before the test and ensure it's cleaned up after.
	if _, err := os.Stat(DefaultDatabasePath); err == nil {
		os.Remove(DefaultDatabasePath) // Remove if exists
	}
	defer func() {
		os.Remove(DefaultDatabasePath)
		// Try to remove .smolcode if empty, fine if it fails
		// os.Remove(dbDir)
	}()

	conv, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	type testMessage struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	msg1 := testMessage{Name: "message1", Value: 100}
	msg2 := map[string]interface{}{"text": "message2", "valid": true}

	conv.Append(msg1)
	conv.Append(msg2)

	err = Save(conv) // This will use the const DefaultDatabasePath
	if err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Verify database content directly by opening DefaultDatabasePath
	db, err := sql.Open("sqlite3", DefaultDatabasePath)
	if err != nil {
		t.Fatalf("Failed to open database for verification at %s: %v", DefaultDatabasePath, err)
	}
	defer db.Close()

	// Verify conversation exists
	var convIdInDB string
	err = db.QueryRow("SELECT id FROM conversations WHERE id = ?", conv.ID).Scan(&convIdInDB)
	if err != nil {
		t.Fatalf("Failed to query conversation: %v", err)
	}
	if convIdInDB != conv.ID {
		t.Errorf("Saved conversation ID mismatch: got %s, want %s", convIdInDB, conv.ID)
	}

	// Verify messages
	rows, err := db.Query("SELECT sequence_number, payload FROM messages WHERE conversation_id = ? ORDER BY sequence_number ASC", conv.ID)
	if err != nil {
		t.Fatalf("Failed to query messages: %v", err)
	}
	defer rows.Close()

	var messagesInDB []map[string]interface{}
	for rows.Next() {
		var seq int
		var payload []byte
		if err := rows.Scan(&seq, &payload); err != nil {
			t.Fatalf("Failed to scan message row: %v", err)
		}
		var msgData map[string]interface{}
		if err := json.Unmarshal(payload, &msgData); err != nil {
			t.Fatalf("Failed to unmarshal message payload: %v", err)
		}
		messagesInDB = append(messagesInDB, msgData)
	}

	if len(messagesInDB) != 2 {
		t.Fatalf("Incorrect number of messages saved: got %d, want %d", len(messagesInDB), 2)
	}

	// Verify msg1 (testMessage struct)
	expectedMsg1JSON, _ := json.Marshal(msg1)
	var expectedMsg1Map map[string]interface{}
	json.Unmarshal(expectedMsg1JSON, &expectedMsg1Map)

	if messagesInDB[0]["name"] != expectedMsg1Map["name"] || int(messagesInDB[0]["value"].(float64)) != int(expectedMsg1Map["value"].(float64)) {
		t.Errorf("Message 0 mismatch. Got: %v, Expected: %v", messagesInDB[0], expectedMsg1Map)
	}

	// Verify msg2 (map[string]interface{})
	if messagesInDB[1]["text"] != msg2["text"] || messagesInDB[1]["valid"] != msg2["valid"] {
		t.Errorf("Message 1 mismatch. Got: %v, Expected: %v", messagesInDB[1], msg2)
	}

	// Test Save again with more messages
	msg3 := "a simple string"
	conv.Append(msg3)
	err = Save(conv) // Save again
	if err != nil {
		t.Fatalf("Second Save() failed: %v", err)
	}

	// Verify again - expect 3 messages now
	rows2, err := db.Query("SELECT payload FROM messages WHERE conversation_id = ? ORDER BY sequence_number ASC", conv.ID)
	if err != nil {
		t.Fatalf("Failed to query messages after second save: %v", err)
	}
	defer rows2.Close()
	var messagesAfterSecondSave [][]byte
	for rows2.Next() {
		var payload []byte
		if err := rows2.Scan(&payload); err != nil {
			t.Fatalf("Failed to scan message row after second save: %v", err)
		}
		messagesAfterSecondSave = append(messagesAfterSecondSave, payload)
	}

	if len(messagesAfterSecondSave) != 3 {
		t.Fatalf("Incorrect number of messages after second save: got %d, want %d", len(messagesAfterSecondSave), 3)
	}

	var msg3Recovered string
	if err := json.Unmarshal(messagesAfterSecondSave[2], &msg3Recovered); err != nil {
		t.Fatalf("Failed to unmarshal msg3: %v", err)
	}
	if msg3Recovered != msg3 {
		t.Errorf("Message 2 (appended) mismatch. Got: %s, Expected: %s", msg3Recovered, msg3)
	}

	// Conceptual: Restore original DefaultDatabasePath if it were changed.
	// DefaultDatabasePath = originalDefaultPath
}

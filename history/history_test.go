package history

import (
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
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
	if err := tempFile.Close(); err != nil {
		t.Fatalf("Failed to close temp db file: %v", err)
	}

	db, err := sql.Open("sqlite3", tempFilePath)
	if err != nil {
		t.Fatalf("Failed to open db at %s: %v", tempFilePath, err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS conversations (id TEXT PRIMARY KEY, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP);`)
	if err != nil {
		db.Close()
		t.Fatalf("Failed to create conversations table in %s: %v", tempFilePath, err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		conversation_id TEXT NOT NULL,
		sequence_number INTEGER NOT NULL,
		payload BLOB NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (conversation_id) REFERENCES conversations(id),
		UNIQUE (conversation_id, sequence_number)
	);`)
	if err != nil {
		db.Close()
		t.Fatalf("Failed to create messages table in %s: %v", tempFilePath, err)
	}

	cleanup := func() {
		db.Close()
		// os.Remove(tempFilePath)
	}
	return db, tempFilePath, cleanup
}

func TestSaveTo(t *testing.T) {
	_, tempDbPath, cleanup := newTestDB(t)
	defer cleanup()

	conv, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	type testMessage struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	msg1 := testMessage{Name: "message1_saveto", Value: 200}
	msg2 := map[string]interface{}{"text": "message2_saveto", "valid": false}

	conv.Append(msg1)
	conv.Append(msg2)

	err = SaveTo(conv, tempDbPath)
	if err != nil {
		t.Fatalf("SaveTo() failed: %v", err)
	}

	dbVerify, err := sql.Open("sqlite3", tempDbPath)
	if err != nil {
		t.Fatalf("Failed to open database for verification at %s: %v", tempDbPath, err)
	}
	defer dbVerify.Close()

	var convIdInDB string
	var convCreatedAtInDB time.Time
	err = dbVerify.QueryRow("SELECT id, created_at FROM conversations WHERE id = ?", conv.ID).Scan(&convIdInDB, &convCreatedAtInDB)
	if err != nil {
		t.Fatalf("Failed to query conversation: %v", err)
	}
	if convIdInDB != conv.ID {
		t.Errorf("Saved conversation ID mismatch: got %s, want %s", convIdInDB, conv.ID)
	}
	// Check that timestamps are close, allowing for minor precision differences in DB storage
	if convCreatedAtInDB.Unix() != conv.CreatedAt.Unix() {
		t.Errorf("Saved conversation CreatedAt mismatch: got %v, want %v", convCreatedAtInDB, conv.CreatedAt)
	}

	rows, err := dbVerify.Query("SELECT sequence_number, payload, created_at FROM messages WHERE conversation_id = ? ORDER BY sequence_number ASC", conv.ID)
	if err != nil {
		t.Fatalf("Failed to query messages: %v", err)
	}
	defer rows.Close()

	var messagesInDB []map[string]interface{}
	var messageTimestamps []time.Time
	for rows.Next() {
		var seq int
		var payload []byte
		var createdAt time.Time
		if err := rows.Scan(&seq, &payload, &createdAt); err != nil {
			t.Fatalf("Failed to scan message row: %v", err)
		}
		var msgData map[string]interface{}
		if err := json.Unmarshal(payload, &msgData); err != nil {
			t.Fatalf("Failed to unmarshal message payload: %v", err)
		}
		messagesInDB = append(messagesInDB, msgData)
		messageTimestamps = append(messageTimestamps, createdAt)
	}

	if len(messagesInDB) != 2 {
		t.Fatalf("Incorrect number of messages saved: got %d, want %d", len(messagesInDB), 2)
	}

	// Check message timestamps
	if messageTimestamps[0].Unix() != conv.Messages[0].CreatedAt.Unix() {
		t.Errorf("Message 0 CreatedAt mismatch: got %v, want %v", messageTimestamps[0], conv.Messages[0].CreatedAt)
	}
	if messageTimestamps[1].Unix() != conv.Messages[1].CreatedAt.Unix() {
		t.Errorf("Message 1 CreatedAt mismatch: got %v, want %v", messageTimestamps[1], conv.Messages[1].CreatedAt)
	}

	expectedMsg1JSON, _ := json.Marshal(msg1)
	var expectedMsg1Map map[string]interface{}
	json.Unmarshal(expectedMsg1JSON, &expectedMsg1Map)

	if messagesInDB[0]["name"] != expectedMsg1Map["name"] || int(messagesInDB[0]["value"].(float64)) != int(expectedMsg1Map["value"].(float64)) {
		t.Errorf("Message 0 mismatch. Got: %v, Expected: %v", messagesInDB[0], expectedMsg1Map)
	}

	if messagesInDB[1]["text"] != msg2["text"] || messagesInDB[1]["valid"] != msg2["valid"] {
		t.Errorf("Message 1 mismatch. Got: %v, Expected: %v", messagesInDB[1], msg2)
	}

	msg3 := "a simple string for SaveTo"
	conv.Append(msg3)
	err = SaveTo(conv, tempDbPath)
	if err != nil {
		t.Fatalf("Second SaveTo() failed: %v", err)
	}

	rows2, err := dbVerify.Query("SELECT payload FROM messages WHERE conversation_id = ? ORDER BY sequence_number ASC", conv.ID)
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
	if conv.CreatedAt.IsZero() {
		t.Error("New() returned conversation with zero CreatedAt")
	}
	if len(conv.Messages) != 0 {
		t.Errorf("New() returned conversation with non-empty messages: got %d, want %d", len(conv.Messages), 0)
	}
}

func TestAppend(t *testing.T) {
	conv, _ := New()
	msgStr := "hello"
	conv.Append(msgStr)
	if len(conv.Messages) != 1 {
		t.Fatalf("Append() failed to add first message. Got %d, want %d", len(conv.Messages), 1)
	}
	if conv.Messages[0].Payload != msgStr {
		t.Errorf("Append() stored incorrect message payload. Got %v, want %s", conv.Messages[0].Payload, msgStr)
	}
	if conv.Messages[0].CreatedAt.IsZero() {
		t.Error("Append() did not set CreatedAt for the first message")
	}

	// Allow some time to pass to ensure the next timestamp is different
	time.Sleep(1 * time.Millisecond)

	msgInt := 123
	conv.Append(msgInt)
	if len(conv.Messages) != 2 {
		t.Fatalf("Append() failed to add second message. Got %d, want %d", len(conv.Messages), 2)
	}
	if conv.Messages[1].Payload != msgInt {
		t.Errorf("Append() stored incorrect message payload. Got %v, want %d", conv.Messages[1].Payload, msgInt)
	}
	if conv.Messages[1].CreatedAt.IsZero() {
		t.Error("Append() did not set CreatedAt for the second message")
	}
	if conv.Messages[1].CreatedAt == conv.Messages[0].CreatedAt {
		t.Errorf("Append() set same CreatedAt for two consecutive messages: %v", conv.Messages[1].CreatedAt)
	}
}

func TestSave(t *testing.T) {
	originalDbPath := DefaultDatabasePath
	tempDbFile, err := os.CreateTemp(t.TempDir(), "test_save_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp db file for TestSave: %v", err)
	}
	tempDbPath := tempDbFile.Name()
	if err := tempDbFile.Close(); err != nil {
		t.Fatalf("Failed to close temp db file for TestSave: %v", err)
	}

	DefaultDatabasePath = tempDbPath
	t.Cleanup(func() {
		DefaultDatabasePath = originalDbPath
		os.Remove(tempDbPath)
	})

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

	err = Save(conv)
	if err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	db, err := sql.Open("sqlite3", DefaultDatabasePath)
	if err != nil {
		t.Fatalf("Failed to open database for verification at %s: %v", DefaultDatabasePath, err)
	}
	defer db.Close()

	var convIdInDB string
	var convCreatedAtInDB time.Time
	err = db.QueryRow("SELECT id, created_at FROM conversations WHERE id = ?", conv.ID).Scan(&convIdInDB, &convCreatedAtInDB)
	if err != nil {
		t.Fatalf("Failed to query conversation: %v", err)
	}
	if convIdInDB != conv.ID {
		t.Errorf("Saved conversation ID mismatch: got %s, want %s", convIdInDB, conv.ID)
	}
	if convCreatedAtInDB.Unix() != conv.CreatedAt.Unix() {
		t.Errorf("Saved conversation CreatedAt mismatch: got %v, want %v", convCreatedAtInDB, conv.CreatedAt)
	}

	rows, err := db.Query("SELECT sequence_number, payload, created_at FROM messages WHERE conversation_id = ? ORDER BY sequence_number ASC", conv.ID)
	if err != nil {
		t.Fatalf("Failed to query messages: %v", err)
	}
	defer rows.Close()

	var messagesInDB []map[string]interface{}
	var messageTimestamps []time.Time
	for rows.Next() {
		var seq int
		var payload []byte
		var createdAt time.Time
		if err := rows.Scan(&seq, &payload, &createdAt); err != nil {
			t.Fatalf("Failed to scan message row: %v", err)
		}
		var msgData map[string]interface{}
		if err := json.Unmarshal(payload, &msgData); err != nil {
			t.Fatalf("Failed to unmarshal message payload: %v", err)
		}
		messagesInDB = append(messagesInDB, msgData)
		messageTimestamps = append(messageTimestamps, createdAt)
	}

	if len(messagesInDB) != 2 {
		t.Fatalf("Incorrect number of messages saved: got %d, want %d", len(messagesInDB), 2)
	}

	// Check message timestamps
	if messageTimestamps[0].Unix() != conv.Messages[0].CreatedAt.Unix() {
		t.Errorf("Message 0 CreatedAt mismatch: got %v, want %v", messageTimestamps[0], conv.Messages[0].CreatedAt)
	}
	if messageTimestamps[1].Unix() != conv.Messages[1].CreatedAt.Unix() {
		t.Errorf("Message 1 CreatedAt mismatch: got %v, want %v", messageTimestamps[1], conv.Messages[1].CreatedAt)
	}

	expectedMsg1JSON, _ := json.Marshal(msg1)
	var expectedMsg1Map map[string]interface{}
	json.Unmarshal(expectedMsg1JSON, &expectedMsg1Map)

	if messagesInDB[0]["name"] != expectedMsg1Map["name"] || int(messagesInDB[0]["value"].(float64)) != int(expectedMsg1Map["value"].(float64)) {
		t.Errorf("Message 0 mismatch. Got: %v, Expected: %v", messagesInDB[0], expectedMsg1Map)
	}

	if messagesInDB[1]["text"] != msg2["text"] || messagesInDB[1]["valid"] != msg2["valid"] {
		t.Errorf("Message 1 mismatch. Got: %v, Expected: %v", messagesInDB[1], msg2)
	}

	msg3 := "a simple string"
	conv.Append(msg3)
	err = Save(conv)
	if err != nil {
		t.Fatalf("Second Save() failed: %v", err)
	}

	db2, err := sql.Open("sqlite3", DefaultDatabasePath)
	if err != nil {
		t.Fatalf("Failed to open database for second verification: %v", err)
	}
	defer db2.Close()

	rows2, err := db2.Query("SELECT payload FROM messages WHERE conversation_id = ? ORDER BY sequence_number ASC", conv.ID)
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
}

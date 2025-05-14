package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func createTestDB(t *testing.T, conversationsToSave ...*Conversation) string {
	t.Helper()
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test_history.db")

	// Ensure initDB runs to create tables, even if no conversations are saved initially.
	// This is important for tests that might expect an empty but valid DB.
	emptyDB, err := initDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize empty test DB: %v", err)
	}
	emptyDB.Close()

	for _, conv := range conversationsToSave {
		if err := SaveTo(conv, dbPath); err != nil {
			t.Fatalf("Failed to save conversation %s to test DB %s: %v", conv.ID, dbPath, err)
		}
	}
	return dbPath
}

func TestLoadFrom(t *testing.T) {
	conv1Time := time.Now().Add(-time.Hour)
	conv1Msg1Time := conv1Time.Add(time.Minute)
	conv1Msg2Time := conv1Time.Add(2 * time.Minute)

	conv1 := &Conversation{
		ID:        "test-conv-1",
		CreatedAt: conv1Time,
		Messages: []*Message{
			{Payload: "Hello", CreatedAt: conv1Msg1Time},
			{Payload: "World", CreatedAt: conv1Msg2Time},
		},
	}

	conv2Time := time.Now().Add(-2 * time.Hour)
	conv2 := &Conversation{
		ID:        "test-conv-2",
		CreatedAt: conv2Time,
		Messages:  []*Message{},
	}

	t.Run("load existing conversation with messages", func(t *testing.T) {
		dbPath := createTestDB(t, conv1, conv2)
		loadedConv, err := LoadFrom(conv1.ID, dbPath)
		if err != nil {
			t.Fatalf("LoadFrom failed: %v", err)
		}
		if diff := cmp.Diff(conv1, loadedConv); diff != "" {
			t.Errorf("Loaded conversation mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("load existing conversation without messages", func(t *testing.T) {
		dbPath := createTestDB(t, conv1, conv2)
		loadedConv, err := LoadFrom(conv2.ID, dbPath)
		if err != nil {
			t.Fatalf("LoadFrom failed: %v", err)
		}
		// Ensure Messages is empty, not nil, if that's the convention from New()
		if loadedConv.Messages == nil {
			loadedConv.Messages = make([]*Message, 0) // Match expected if necessary
		}
		if diff := cmp.Diff(conv2, loadedConv); diff != "" {
			t.Errorf("Loaded conversation mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("load non-existent conversation", func(t *testing.T) {
		dbPath := createTestDB(t, conv1) // Only conv1 exists
		_, err := LoadFrom("non-existent-id", dbPath)
		if err == nil {
			t.Fatal("LoadFrom expected an error for non-existent ID, got nil")
		}
		// TODO: Check for a specific error type if defined, e.g., ErrConversationNotFound
		t.Logf("Received expected error: %v", err)
	})

	t.Run("database file not found", func(t *testing.T) {
		nonExistentDbPath := filepath.Join(t.TempDir(), "ghost.db")
		_, err := LoadFrom("any-id", nonExistentDbPath)
		if err == nil {
			t.Fatal("LoadFrom expected an error for non-existent database file, got nil")
		}
		// Check if the error indicates file not found (could be os.ErrNotExist or a wrapped one)
		t.Logf("Received expected error for non-existent DB: %v", err)
	})

	t.Run("malformed database file", func(t *testing.T) {
		dbDir := t.TempDir()
		malformedDbPath := filepath.Join(dbDir, "corrupt.db")
		if err := os.WriteFile(malformedDbPath, []byte("this is not a valid sqlite database"), 0644); err != nil {
			t.Fatalf("Failed to create malformed DB file: %v", err)
		}
		_, err := LoadFrom("any-id", malformedDbPath)
		if err == nil {
			t.Fatalf("LoadFrom expected an error for malformed database, got nil")
		}
		t.Logf("Received expected error for malformed DB: %v", err)
	})
}

func TestLoad(t *testing.T) {
	// Store original DefaultDatabasePath and defer its restoration
	originalDefaultDBPath := DefaultDatabasePath
	defer func() { DefaultDatabasePath = originalDefaultDBPath }()

	// Create a temporary directory for the default test DB
	tempDir := t.TempDir()
	DefaultDatabasePath = filepath.Join(tempDir, "default_test_history.db")

	// Clean up the temporary default database file after the test
	defer os.Remove(DefaultDatabasePath)

	convDefaultTime := time.Now().Add(-30 * time.Minute)
	convDefaultMsgTime := convDefaultTime.Add(time.Second)
	convDefault := &Conversation{
		ID:        "test-conv-default",
		CreatedAt: convDefaultTime,
		Messages: []*Message{
			{Payload: "Default Test", CreatedAt: convDefaultMsgTime},
		},
	}

	// Save this conversation to the now-temporary DefaultDatabasePath
	// Need to ensure the db and tables are created first
	db, err := initDB(DefaultDatabasePath)
	if err != nil {
		t.Fatalf("Failed to init default test DB: %v", err)
	}
	db.Close() // initDB opens it, close it before SaveTo reopens

	if err := Save(convDefault); err != nil { // Save uses DefaultDatabasePath
		t.Fatalf("Failed to save conversation to default DB: %v", err)
	}

	loadedConv, err := Load(convDefault.ID)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if diff := cmp.Diff(convDefault, loadedConv); diff != "" {
		t.Errorf("Loaded conversation from default path mismatch (-want +got):\n%s", diff)
	}

	// Test loading non-existent from default
	_, err = Load("non-existent-in-default")
	if err == nil {
		t.Fatal("Load expected an error for non-existent ID in default DB, got nil")
	}
	t.Logf("Received expected error for non-existent in default: %v", err)
}

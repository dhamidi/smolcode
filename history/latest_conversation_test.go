package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Helper to create a test DB and save conversations
func setupTestDB(t *testing.T, conversationsToCreate []Conversation) string {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_history.db")

	for _, conv := range conversationsToCreate {
		// Ensure CreatedAt is set if not provided for ordering
		if conv.CreatedAt.IsZero() {
			conv.CreatedAt = time.Now()
			// Add a small delay to ensure distinct created_at for ordering tests
			time.Sleep(10 * time.Millisecond)
		}
		err := SaveTo(&conv, dbPath) // SaveTo uses initDB internally
		if err != nil {
			t.Fatalf("Failed to save conversation %s for test setup: %v", conv.ID, err)
		}
	}
	return dbPath
}

func TestGetLatestConversationID(t *testing.T) {
	tests := []struct {
		name                  string
		conversationsToCreate []Conversation
		expectedID            string
		expectedErr           error
	}{
		{
			name:                  "empty database",
			conversationsToCreate: []Conversation{},
			expectedID:            "",
			expectedErr:           ErrConversationNotFound,
		},
		{
			name: "single conversation",
			conversationsToCreate: []Conversation{
				{ID: "conv1", CreatedAt: time.Now().Add(-time.Hour)}, // Ensure CreatedAt is explicitly set for predictability
			},
			expectedID:  "conv1",
			expectedErr: nil,
		},
		{
			name: "multiple conversations, latest is newest",
			conversationsToCreate: []Conversation{
				{ID: "conv1", CreatedAt: time.Now().Add(-2 * time.Hour)},
				{ID: "conv2", CreatedAt: time.Now().Add(-1 * time.Hour)}, // conv2 is latest
				{ID: "conv3", CreatedAt: time.Now().Add(-3 * time.Hour)},
			},
			expectedID:  "conv2",
			expectedErr: nil,
		},
		{
			name: "multiple conversations, ensure correct ordering",
			conversationsToCreate: []Conversation{
				{ID: "oldest", CreatedAt: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
				{ID: "newest", CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
				{ID: "middle", CreatedAt: time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)},
			},
			expectedID:  "newest",
			expectedErr: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dbPath := setupTestDB(t, tc.conversationsToCreate)

			actualID, actualErr := GetLatestConversationID(dbPath)

			if actualErr != tc.expectedErr {
				// Special handling for sql.ErrNoRows as it might be wrapped by fmt.Errorf in some implementations
				// However, our GetLatestConversationID returns it directly.
				t.Errorf("GetLatestConversationID() error = %v, wantErr %v", actualErr, tc.expectedErr)
			}
			if actualID != tc.expectedID {
				t.Errorf("GetLatestConversationID() id = %s, want %s", actualID, tc.expectedID)
			}

			// Clean up the test database file if not using t.TempDir(), but t.TempDir() handles it.
			// os.Remove(dbPath)
		})
	}
}

// TestGetLatestConversationID_DBNotExists tests behavior when the DB file doesn't exist.
// initDB (called by GetLatestConversationID) should create it.
func TestGetLatestConversationID_DBNotExists(t *testing.T) {
	tempDir := t.TempDir()
	nonExistentDBPath := filepath.Join(tempDir, "non_existent.db")

	// Ensure it really doesn't exist first (though TempDir should be clean)
	_, err := os.Stat(nonExistentDBPath)
	if !os.IsNotExist(err) {
		t.Fatalf("DB file %s unexpectedly exists or other error: %v", nonExistentDBPath, err)
	}

	id, err := GetLatestConversationID(nonExistentDBPath)
	if err != ErrConversationNotFound {
		t.Errorf("Expected sql.ErrNoRows when DB is new and empty, got %v", err)
	}
	if id != "" {
		t.Errorf("Expected empty ID when DB is new and empty, got %s", id)
	}
}

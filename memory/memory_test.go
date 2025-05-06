package memory

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// Helper function to setup a temporary test database
func setupTestDB(t *testing.T) (*sql.DB, func()) {
	// Use a temporary directory for the test database
	tempDir := t.TempDir()
	originalDbDir := dbDir   // Save original
	originalDbPath := dbPath // Save original

	dbDir = filepath.Join(tempDir, ".smolcode_test_data") // Override package-level var for this test
	dbPath = filepath.Join(dbDir, dbFileName)

	db, err := InitDB()
	if err != nil {
		t.Fatalf("InitDB() failed: %v", err)
	}

	cleanup := func() {
		db.Close()
		dbDir = originalDbDir   // Restore original
		dbPath = originalDbPath // Restore original
		// The t.TempDir() will handle removing the directory and its contents
	}

	return db, cleanup
}

func TestInitDB(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	if db == nil {
		t.Fatal("Expected db to be non-nil")
	}

	// Check if tables were created (basic check)
	_, err := db.Query("SELECT id, content FROM facts LIMIT 1")
	if err != nil {
		t.Errorf("Querying facts table failed: %v", err)
	}

	_, err = db.Query("SELECT content FROM facts_fts LIMIT 1")
	if err != nil {
		t.Errorf("Querying facts_fts table failed: %v", err)
	}
}

func TestAddFact(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	testID := "test-fact-1"
	testContent := "This is a test fact for SQLite FTS5."

	err := AddFact(db, testID, testContent)
	if err != nil {
		t.Fatalf("AddFact() failed: %v", err)
	}

	// Verify the fact was added to the main table
	var content string
	err = db.QueryRow("SELECT content FROM facts WHERE id = ?", testID).Scan(&content)
	if err != nil {
		t.Fatalf("Failed to retrieve fact from facts table: %v", err)
	}
	if content != testContent {
		t.Errorf("Expected content '%s', got '%s'", testContent, content)
	}

	// Verify the fact was added to the FTS table (indirectly, by searching)
	// This also tests the triggers somewhat.
	rows, err := db.Query("SELECT rowid FROM facts_fts WHERE facts_fts MATCH ?", "SQLite")
	if err != nil {
		t.Fatalf("Failed to query facts_fts: %v", err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		var rowid string
		if err := rows.Scan(&rowid); err != nil {
			t.Fatalf("Failed to scan rowid from facts_fts: %v", err)
		}
		if rowid == testID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Fact with ID '%s' not found in facts_fts after AddFact via MATCH", testID)
	}

	// Test updating an existing fact
	updatedContent := "This is an updated test fact."
	err = AddFact(db, testID, updatedContent)
	if err != nil {
		t.Fatalf("AddFact() for update failed: %v", err)
	}
	err = db.QueryRow("SELECT content FROM facts WHERE id = ?", testID).Scan(&content)
	if err != nil {
		t.Fatalf("Failed to retrieve updated fact: %v", err)
	}
	if content != updatedContent {
		t.Errorf("Expected updated content '%s', got '%s'", updatedContent, content)
	}
}

func TestSearchFacts(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	facts := []Fact{
		{ID: "fact1", Content: "The quick brown fox jumps over the lazy dog"},
		{ID: "fact2", Content: "SQLite is a C-language library that implements a small, fast, self-contained, high-reliability, full-featured, SQL database engine."},
		{ID: "fact3", Content: "FTS5 is an SQLite virtual table module that provides full-text search capabilities."},
		{ID: "fact4", Content: "Another fact about a quick brown animal, perhaps a squirrel."},
	}

	for _, f := range facts {
		if err := AddFact(db, f.ID, f.Content); err != nil {
			t.Fatalf("Failed to add fact %s: %v", f.ID, err)
		}
	}

	t.Run("Search for single word", func(t *testing.T) {
		results, err := SearchFacts(db, "SQLite")
		if err != nil {
			t.Fatalf("SearchFacts failed: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("Expected 2 results for 'SQLite', got %d", len(results))
		}
		// Check if specific facts are present (order might vary due to rank)
		foundFact2 := false
		foundFact3 := false
		for _, r := range results {
			if r.ID == "fact2" {
				foundFact2 = true
			}
			if r.ID == "fact3" {
				foundFact3 = true
			}
		}
		if !foundFact2 || !foundFact3 {
			t.Errorf("Expected fact2 and fact3 in results for 'SQLite', got: %+v", results)
		}
	})

	t.Run("Search for multiple words", func(t *testing.T) {
		results, err := SearchFacts(db, "quick brown")
		if err != nil {
			t.Fatalf("SearchFacts failed: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("Expected 2 results for 'quick brown', got %d. Results: %+v", len(results), results)
		}
	})

	t.Run("Search for unique word", func(t *testing.T) {
		results, err := SearchFacts(db, "FTS5")
		if err != nil {
			t.Fatalf("SearchFacts failed: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("Expected 1 result for 'FTS5', got %d", len(results))
		}
		if results[0].ID != "fact3" {
			t.Errorf("Expected fact3 for 'FTS5', got %s", results[0].ID)
		}
	})

	t.Run("Search with no matches", func(t *testing.T) {
		results, err := SearchFacts(db, "nonexistentXYZ")
		if err != nil {
			t.Fatalf("SearchFacts failed: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("Expected 0 results for 'nonexistentXYZ', got %d", len(results))
		}
	})

	t.Run("Search with empty query", func(t *testing.T) {
		results, err := SearchFacts(db, "")
		if err != nil {
			t.Fatalf("SearchFacts for empty query failed: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("Expected 0 results for empty query, got %d", len(results))
		}
	})
}

package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestEscapeFTSQueryString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", ""},
		{"simple string", "hello", "hello"},
		{"string with spaces", "hello world", "hello world"},
		{"string with double quotes", "hello \"world\"", "hello \"\"world\"\""},
		{"string with single quotes", "hello 'world'", "hello 'world'"},
		{"string with path characters", "path/to/file.txt", "path/to/file.txt"},
		{"string with mixed special chars", "a\"b/c'd-e*f", "a\"\"b/c'd-e*f"},
		{"only double quotes", `""`, `""""`},
		{"FTS keyword AND", "AND", "AND"},
		{"FTS keyword OR", "OR", "OR"},
		{"FTS keyword NOT", "NOT", "NOT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := escapeFTSQueryString(tt.input); got != tt.want {
				t.Errorf("escapeFTSQueryString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func setupTestDB(t *testing.T) (*MemoryManager, func()) {
	tempDir, err := os.MkdirTemp("", "memory_test_")
	if err != nil {
		t.Fatalf("Failed to create temp dir for test DB: %v", err)
	}
	dbPath := filepath.Join(tempDir, "test_mem.db")

	mm, err := New(dbPath)
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("New() failed: %v", err)
	}

	cleanup := func() {
		mm.Close()
		os.RemoveAll(tempDir)
	}

	return mm, cleanup
}

func TestSearchMemoryWithSpecialChars(t *testing.T) {
	mm, cleanup := setupTestDB(t)
	defer cleanup()

	testMemories := []struct {
		id      string
		content string
	}{
		{"id1", "This is a normal file."},
		{"id2", "This file is at path/to/document.txt"},
		{"id3", "Another file: specific-file-name.md"},
		{"id4", "Content with \"quotes\" in it."},
		{"id5", "A file named a*b.txt"},
		{"id6", "A file with 'single quotes'"},
		{"id7", "File with AND OR NOT keywords"},
		{"id8", "File path with spaces: /mnt/my docs/report.docx"},
	}

	for _, mem := range testMemories {
		if err := mm.AddMemory(mem.id, mem.content); err != nil {
			t.Fatalf("AddMemory(%s, %s) failed: %v", mem.id, mem.content, err)
		}
	}

	searchTests := []struct {
		name          string
		query         string
		expectedIDs   []string // Order might matter if rank is consistent, but for now check presence
		expectedCount int
	}{
		{"search normal file", "normal file", []string{"id1"}, 1},
		{"search path with slashes", "path/to/document.txt", []string{"id2"}, 1},
		{"search specific file name", "specific-file-name.md", []string{"id3"}, 1},
		{"search content with quotes", "content with \"quotes\" in it.", []string{"id4"}, 1},
		{"search exact content with quotes", "\"Content with \"\"quotes\"\" in it.\"", []string{"id4"}, 1},       //This should be 1, FTS5 matches the content despite heavy escaping.
		{"search content with escaped quotes for FTS", "Content with \"\"quotes\"\" in it.", []string{"id4"}, 1}, //This should be 1 because the query is escaped by our func.
		{"search file with asterisk", "a*b.txt", []string{"id5"}, 1},
		{"search file with single quotes", "A file with 'single quotes'", []string{"id6"}, 1},
		{"search file with FTS keywords", "AND OR NOT", []string{"id7"}, 1},
		{"search path with spaces", "/mnt/my docs/report.docx", []string{"id8"}, 1},
		{"search non-existent", "nonexistent", []string{}, 0},
		{"search part of path", "path/to", []string{"id2"}, 1}, // FTS might tokenize this. Exact phrase needed.
		{"search empty string", "", []string{}, 0},             // We escape to "" which likely matches nothing.
	}

	for _, st := range searchTests {
		t.Run(st.name, func(t *testing.T) {
			results, err := mm.SearchMemory(st.query)
			if err != nil {
				t.Errorf("SearchMemory(%s) failed: %v", st.query, err)
				return
			}

			if len(results) != st.expectedCount {
				t.Errorf("SearchMemory(%s): got %d results, want %d. Results: %v", st.query, len(results), st.expectedCount, results)
			}

			if st.expectedCount > 0 {
				foundIDs := make(map[string]bool)
				for _, res := range results {
					foundIDs[res.ID] = true
				}
				for _, expectedID := range st.expectedIDs {
					if !foundIDs[expectedID] {
						t.Errorf("SearchMemory(%s): expected to find ID %s, but not found in results %v", st.query, expectedID, results)
					}
				}
			}
		})
	}

	// Test specifically how FTS treats the escaped query "path/to/document.txt"
	// It should find ID2
	t.Run("search exact path direct FTS expectation", func(t *testing.T) {
		// The user query is "path/to/document.txt"
		// Our function will turn this into ""path/to/document.txt"" for the MATCH clause
		// SQLite FTS should treat this as a phrase search for "path/to/document.txt"
		query := "path/to/document.txt"
		results, err := mm.SearchMemory(query)
		if err != nil {
			t.Fatalf("SearchMemory for exact path failed: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("Expected 1 result for query '%s', got %d. Results: %v", query, len(results), results)
		}
		if results[0].ID != "id2" {
			t.Errorf("Expected ID id2 for query '%s', got %s", query, results[0].ID)
		}
	})

	// Test specifically that searching for "content" does not find "Content with "quotes" in it."
	// but searching for "Content with "quotes" in it." (with our escaping) does.
	t.Run("distinguish partial vs full quoted content", func(t *testing.T) {
		queryNoQuotes := "Content with quotes in it."
		results, err := mm.SearchMemory(queryNoQuotes)
		if err != nil {
			t.Fatalf("SearchMemory for '%s' failed: %v", queryNoQuotes, err)
		}
		// This should find id4 because FTS tokenizes "Content", "with", "quotes", "in", "it" and matches.
		// Our escaping aims to make the *user's input* literal.
		// If user inputs "Content with quotes in it." it becomes ""Content with quotes in it.""
		// If user inputs "Content with \"quotes\" in it." it becomes ""Content with \"\"quotes\"\" in it.""
		// The document is "Content with \"quotes\" in it."
		// FTS tokenizes this as "Content", "with", "quotes", "in", "it".
		// So searching for "Content with quotes in it." (which becomes ""Content with quotes in it."") WILL match.
		// This test case needs rethinking in light of how FTS tokenizes *indexed* content versus *query* content.

		// Revised expectation: Searching for "Content with quotes in it." (no literal quotes in query)
		// becomes ""Content with quotes in it.""
		// The document "Content with \"quotes\" in it." is tokenized as "Content", "with", "quotes", "in", "it."
		// The FTS query ""Content with quotes in it."" will match these tokens in sequence.
		if len(results) != 1 || results[0].ID != "id4" {
			t.Errorf("SearchMemory for '%s': expected 1 result (id4), got %d. Results: %v", queryNoQuotes, len(results), results)
		}

		queryWithQuotes := "Content with \"quotes\" in it." // User types this
		// Internally becomes: ""Content with \"\"quotes\"\" in it.""
		results, err = mm.SearchMemory(queryWithQuotes)
		if err != nil {
			t.Fatalf("SearchMemory for '%s' failed: %v", queryWithQuotes, err)
		}
		if len(results) != 1 {
			for _, res := range results {
				fmt.Printf("Found: ID=%s, Content=%s\n", res.ID, res.Content)
			}
			t.Fatalf("SearchMemory for query with quotes '%s': expected 1 result (id4), got %d. Results: %v", queryWithQuotes, len(results), results)
		}
		if results[0].ID != "id4" {
			t.Errorf("SearchMemory for query with quotes '%s': expected ID id4, got %s", queryWithQuotes, results[0].ID)
		}
	})

}

func TestSearchMemoryBuildCommand(t *testing.T) {
	mm, cleanup := setupTestDB(t)
	defer cleanup()

	memories := []struct {
		id      string
		content string
	}{
		{"bc_phrase", "This is a build command."},
		{"bc_separate", "The build process needs a specific command to run."},
		{"b_only", "This document talks about build."},
		{"c_only", "This document talks about command."},
		{"neither", "This is an unrelated document."},
	}

	for _, mem := range memories {
		if err := mm.AddMemory(mem.id, mem.content); err != nil {
			t.Fatalf("AddMemory(%s, %s) failed: %v", mem.id, mem.content, err)
		}
	}

	query := "build command"
	results, err := mm.SearchMemory(query)
	if err != nil {
		t.Fatalf("SearchMemory(%s) failed: %v", query, err)
	}

	if len(results) != 1 {
		t.Errorf("SearchMemory(%s): got %d results, want 1. Results: %v", query, len(results), results)
		// For debugging, print found IDs
		var resultIDs []string
		for _, r := range results {
			resultIDs = append(resultIDs, r.ID)
		}
		t.Logf("Found IDs: %v", resultIDs)
	}

	foundIDs := make(map[string]bool)
	for _, res := range results {
		foundIDs[res.ID] = true
	}

	expectedToFind := []string{"bc_phrase"}
	for _, id := range expectedToFind {
		if !foundIDs[id] {
			t.Errorf("SearchMemory(%s): expected to find ID %s, but not found in results %v", query, id, results)
		}
	}

	expectedToNotFind := []string{"bc_separate", "b_only", "c_only", "neither"}
	for _, id := range expectedToNotFind {
		if foundIDs[id] {
			t.Errorf("SearchMemory(%s): expected NOT to find ID %s, but it was found in results %v", query, id, results)
		}
	}
}

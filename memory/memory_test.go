package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareFTSQuery(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", "\"\""},
		{"all whitespace", "   ", "\"\""},
		{"simple string", "hello", "hello"},
		{"string with spaces", "hello world", "hello world"},
		{"string with internal double quotes", "hello \"world\"", "hello \"\"\"world\"\"\""},
		{"string with single quotes", "hello 'world'", "hello \"'world'\""},
		{"string with path characters", "path/to/file.txt", "\"path/to/file.txt\""},
		{"string with mixed special chars", "a\"b/c'd-e*f", "\"a\"\"b/c'd-e*f\""},
		{"only double quotes", "\"\"", "\"\"\"\"\"\""},
		{"FTS keyword AND", "AND", "\"AND\""},
		{"FTS keyword OR", "OR", "\"OR\""},
		{"FTS keyword NOT", "NOT", "\"NOT\""},
		{"complex and simple terms", "test path/to/file", "test \"path/to/file\""},
		{"term with hyphen", "my-term", "\"my-term\""},
		{"term with apostrophe", "author's", "\"author's\""},
		{"multiple complex", "file.txt report.docx", "\"file.txt\" \"report.docx\""},
		{"keywords and complex", "important AND file.ext", "important \"AND\" \"file.ext\""},
		{"leading/trailing space", "  hello  ", "hello"},
		{"leading/trailing space complex", "  file.txt  ", "\"file.txt\""},
		{"leading/trailing space multi", "  hello world  ", "hello world"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prepareFTSQuery(tt.input); got != tt.want {
				t.Errorf("prepareFTSQuery(%q) = %q, want %q", tt.input, got, tt.want)
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
		// Our function will turn this into "\"path/to/document.txt\"" for the MATCH clause
		// SQLite FTS should treat this as a phrase search for "path/to/document.txt"
		query := "path/to/document.txt"
		results, err := mm.SearchMemory(query)
		if err != nil {
			t.Fatalf("SearchMemory for exact path failed: %v", err)
		}
		if len(results) != 1 { // Reverted to 1
			t.Fatalf("Expected 1 result for query '%s', got %d. Results: %v", query, len(results), results)
		}
		if results[0].ID != "id2" {
			t.Errorf("Expected ID id2 for query '%s', got %s", query, results[0].ID)
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

	if len(results) != 2 { // This is correct for this fix
		t.Errorf("SearchMemory(%s): got %d results, want 2. Results: %v", query, len(results), results) // CORRECTED: want 2
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

	expectedToFind := []string{"bc_phrase", "bc_separate"} // CORRECTED
	for _, id := range expectedToFind {
		if !foundIDs[id] {
			t.Errorf("SearchMemory(%s): expected to find ID %s, but not found in results %v", query, id, results)
		}
	}

	expectedToNotFind := []string{"b_only", "c_only", "neither"} // CORRECTED
	for _, id := range expectedToNotFind {
		if foundIDs[id] {
			t.Errorf("SearchMemory(%s): expected NOT to find ID %s, but it was found in results %v", query, id, results)
		}
	}
}

func TestSearchMemoryWithSpacedTerms(t *testing.T) {
	mm, cleanup := setupTestDB(t)
	defer cleanup()

	memories := []struct {
		id      string
		content string
	}{
		{"spaced_doc", "This document mentions test at the beginning and command at the end."},
		{"other_doc", "This document is about something else entirely."},
	}

	for _, mem := range memories {
		if err := mm.AddMemory(mem.id, mem.content); err != nil {
			t.Fatalf("AddMemory(%s, %s) failed: %v", mem.id, mem.content, err)
		}
	}

	query := "test command"
	results, err := mm.SearchMemory(query)
	if err != nil {
		t.Fatalf("SearchMemory(%s) failed: %v", query, err)
	}

	if len(results) != 1 { // Correct for this test
		t.Errorf("SearchMemory(%s): got %d results, want 1. Results: %v", query, len(results), results)
		var resultIDs []string
		for _, r := range results {
			resultIDs = append(resultIDs, r.ID)
		}
		t.Logf("Found IDs: %v", resultIDs)
	}

	if len(results) == 1 && results[0].ID != "spaced_doc" {
		t.Errorf("SearchMemory(%s): expected to find ID 'spaced_doc', but found %s", query, results[0].ID)
	}

}

package memory

import (
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed schema.sql
var schemaSQL string

var ErrNotFound = errors.New("memory: not found")

type MemoryManager struct {
	db *sql.DB
}

type Memory struct {
	ID      string
	Content string
}

func New(dbPath string) (*MemoryManager, error) {
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory %s: %w", dbDir, err)
	}
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database at %s: %w", dbPath, err)
	}
	if err := initializeSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}
	return &MemoryManager{db: db}, nil
}

func initializeSchema(db *sql.DB) error {
	_, err := db.Exec(schemaSQL)
	if err != nil {
		return fmt.Errorf("failed to execute schema initialization SQL: %w", err)
	}
	return nil
}

func (m *MemoryManager) Close() error {
	if m.db != nil {
		return m.db.Close()
	}
	return nil
}

func (m *MemoryManager) AddMemory(id string, content string) error {
	insertSQL := `
	INSERT INTO memories (id, content)
	VALUES (?, ?)
	ON CONFLICT(id) DO UPDATE SET
		content = excluded.content;
	`
	_, err := m.db.Exec(insertSQL, id, content)
	if err != nil {
		return fmt.Errorf("failed to insert/replace memory with id %s: %w", id, err)
	}
	return nil
}

func (m *MemoryManager) GetMemoryByID(id string) (*Memory, error) {
	querySQL := `SELECT id, content FROM memories WHERE id = ?;`
	row := m.db.QueryRow(querySQL, id)
	mem := &Memory{}
	err := row.Scan(&mem.ID, &mem.Content)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("memory with id '%s': %w", id, ErrNotFound)
		}
		return nil, fmt.Errorf("failed to retrieve memory with id '%s': %w", id, err)
	}
	return mem, nil
}

func isSimpleBarewordChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

func isAllSimpleBarewordChars(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !isSimpleBarewordChar(r) {
			return false
		}
	}
	return true
}

func escapeAndPrepareFTSToken(token string) string {
	uppercaseToken := strings.ToUpper(token)
	// A token is simple if it's composed of only bareword characters
	// AND is NOT an FTS keyword (AND, OR, NOT).
	// FTS Keywords, when used as search terms, must be quoted to be treated literally.
	if isAllSimpleBarewordChars(token) && !(uppercaseToken == "AND" || uppercaseToken == "OR" || uppercaseToken == "NOT") {
		return token
	}

	// Otherwise (it's a keyword, or contains special chars, etc.),
	// treat it as a literal phrase:
	// 1. Escape internal double quotes.
	// 2. Wrap the whole token in double quotes.
	escapedInternalQuotesToken := strings.ReplaceAll(token, "\"", "\"\"")
	return fmt.Sprintf("\"%s\"", escapedInternalQuotesToken)
}

func prepareFTSQuery(query string) string {
	trimmedQuery := strings.TrimSpace(query)
	if trimmedQuery == "" {
		return "\"\""
	}
	terms := strings.Fields(trimmedQuery)
	if len(terms) == 0 {
		return escapeAndPrepareFTSToken(trimmedQuery)
	}
	var preparedTerms []string
	for _, term := range terms {
		preparedTerms = append(preparedTerms, escapeAndPrepareFTSToken(term))
	}
	return strings.Join(preparedTerms, " ")
}

func (m *MemoryManager) SearchMemory(query string) ([]*Memory, error) {
	ftsQuerySQL := `
	SELECT fts.rowid 
	FROM memories_fts AS fts
	WHERE fts.memories_fts MATCH ?
	ORDER BY rank;
	`
	ftsFinalQuery := prepareFTSQuery(query)
	rows, err := m.db.Query(ftsQuerySQL, ftsFinalQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to execute FTS query MATCH '%s': %w", ftsFinalQuery, err)
	}
	defer rows.Close()
	var docIDs []int64
	for rows.Next() {
		var docID int64
		if err := rows.Scan(&docID); err != nil {
			rows.Close()
			return nil, fmt.Errorf("failed to scan docID from FTS results: %w", err)
		}
		docIDs = append(docIDs, docID)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("error iterating FTS docID results: %w", err)
	}
	if len(docIDs) == 0 {
		return []*Memory{}, nil
	}
	var memories []*Memory
	stmtSQL := `SELECT id, content FROM memories WHERE docid = ?;`
	stmt, err := m.db.Prepare(stmtSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare statement to get memory by docid: %w", err)
	}
	defer stmt.Close()
	for _, docID := range docIDs {
		mem := &Memory{}
		err := stmt.QueryRow(docID).Scan(&mem.ID, &mem.Content)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("FTS returned docID %d but no matching memory found (data inconsistency?): %w", docID, err)
			}
			return nil, fmt.Errorf("failed to retrieve memory for docID %d: %w", docID, err)
		}
		memories = append(memories, mem)
	}
	return memories, nil
}

func (m *MemoryManager) Forget(id string) error {
	deleteSQL := `DELETE FROM memories WHERE id = ?;`
	result, err := m.db.Exec(deleteSQL, id)
	if err != nil {
		return fmt.Errorf("failed to delete memory with id %s: %w", id, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		fmt.Printf("Warning: could not get rows affected after deleting memory %s: %v\n", id, err)
	} else if rowsAffected == 0 {
		return fmt.Errorf("memory with id '%s' not found to forget: %w", id, ErrNotFound)
	}
	return nil
}

package memory

import (
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("memory: not found")

func (m *MemoryManager) Forget(id string) error {
	deleteSQL := `DELETE FROM memories WHERE id = ?;`
	result, err := m.db.Exec(deleteSQL, id)
	if err != nil {
		return fmt.Errorf("failed to delete memory with id %s: %w", id, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		// If we can't get rows affected, it's not fatal, but we should log it.
		// The actual deletion might have succeeded.
		fmt.Printf("Warning: could not get rows affected after deleting memory %s: %v\n", id, err)
	} else if rowsAffected == 0 {
		return fmt.Errorf("memory with id '%s' not found to forget: %w", id, ErrNotFound)
	}
	return nil
}

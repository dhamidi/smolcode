package history

import (
	"errors"
	"time"
)

// ErrConversationNotFound is returned when a requested conversation cannot be found.
var ErrConversationNotFound = errors.New("history: conversation not found")

// ConversationMetadata holds summary information about a conversation.
type ConversationMetadata struct {
	ID                string
	LatestMessageTime time.Time
	MessageCount      int
	CreatedAt         time.Time
}

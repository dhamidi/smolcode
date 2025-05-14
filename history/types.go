package history

import "time"

// ConversationMetadata holds summary information about a conversation.
type ConversationMetadata struct {
	ID                string
	LatestMessageTime time.Time
	MessageCount      int
	CreatedAt         time.Time
}

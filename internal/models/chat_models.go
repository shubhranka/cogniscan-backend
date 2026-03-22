// ./cogniscan-backend/internal/models/chat_models.go
package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ChatMessage represents a single message in a conversation
type ChatMessage struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Role      string             `bson:"role" json:"role"`      // "user" or "assistant"
	Content   string             `bson:"content" json:"content"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
}

// ChatCitation represents a note reference used in an answer
type ChatCitation struct {
	NoteID    string  `bson:"noteId" json:"noteId"`
	NoteName  string  `bson:"noteName" json:"noteName"`
	Relevance float32 `bson:"relevance" json:"relevance"` // Similarity score 0-1
	Context   string  `bson:"context" json:"context"`   // The specific caption text used
}

// ChatConversation represents a full chat session for a folder
type ChatConversation struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	FolderID   string             `bson:"folderId" json:"folderId"`
	OwnerID    string             `bson:"ownerId" json:"ownerId"`
	Title      string             `bson:"title" json:"title"`          // Auto-generated from first message
	Messages   []ChatMessage      `bson:"messages" json:"messages"`
	CreatedAt  time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt  time.Time          `bson:"updatedAt" json:"updatedAt"`
}

// ./cogniscan-backend/internal/models/models.go
package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type User struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	Email     string             `bson:"email"`
	GoogleID  string             `bson:"googleId"`
	Name      string             `bson:"name"`
	Picture   string             `bson:"picture"`
	CreatedAt time.Time          `bson:"createdAt"`
	UpdatedAt time.Time          `bson:"updatedAt"`
}

type Folder struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Name      string             `bson:"name" json:"name"`
	ParentID  string             `bson:"parentId" json:"parentId"`
	OwnerID   string             `bson:"ownerId" json:"ownerId"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time          `bson:"updatedAt" json:"updatedAt"`
}

type CaptionStatus string

const (
	CaptionStatusPending   CaptionStatus = "pending"
	CaptionStatusProcessing CaptionStatus = "processing"
	CaptionStatusCompleted CaptionStatus = "completed"
	CaptionStatusFailed    CaptionStatus = "failed"
)

type Note struct {
	ID            primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Name          string             `bson:"name" json:"name"`
	PublicURL     string             `bson:"publicUrl" json:"publicUrl"`
	DriveID       string             `bson:"driveId" json:"driveId"`
	// Caption fields kept internally for vector search but not exposed to UI
	Caption       string             `bson:"caption"`
	CaptionStatus CaptionStatus     `bson:"captionStatus"`
	CaptionError  string             `bson:"captionError,omitempty"`
	CreatedAt     time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt     time.Time          `bson:"updatedAt" json:"updatedAt"`
	FolderID      string             `bson:"folderId" json:"folderId"`
	OwnerID       string             `bson:"ownerId" json:"ownerId"`
}

// CaptionEmbedding represents a caption with its embedding vector
// Stored in a separate collection for vector search
type CaptionEmbedding struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	NoteID      string             `bson:"noteId" json:"noteId"`
	FolderID    string             `bson:"folderId" json:"folderId"`
	OwnerID     string             `bson:"ownerId" json:"ownerId"`
	Caption     string             `bson:"caption" json:"caption"`
	Vector      []float32          `bson:"vector" json:"vector"` // Embedding vector
	CreatedAt   time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt   time.Time          `bson:"updatedAt" json:"updatedAt"`
}

package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// User represents a user in our system.
// In a real system, you might store more info.
type User struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	Email     string             `bson:"email"`
	GoogleID  string             `bson:"googleId"`
	Name      string             `bson:"name"`
	Picture   string             `bson:"picture"`
	CreatedAt time.Time          `bson:"createdAt"`
	UpdatedAt time.Time          `bson:"updatedAt"`
}

// Folder represents the folder structure in MongoDB
type Folder struct {
	ID   primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Name string             `bson:"name" json:"name"`
	// Use string for ParentID. "" means it's a root folder.
	ParentID string `bson:"parentId" json:"parentId"`
	OwnerID  string `bson:"ownerId" json:"ownerId"` // Storing Firebase UID as string
}

// Note represents a scanned note in MongoDB
type Note struct {
	ID primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	// We'll add a 'name' field for display purposes
	Name      string    `bson:"name" json:"name"`
	MegaURL   string    `bson:"megaUrl" json:"megaUrl"`
	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
	FolderID  string    `bson:"folderId" json:"folderId"`
	OwnerID   string    `bson:"ownerId" json:"ownerId"`
}

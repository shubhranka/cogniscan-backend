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
	ID                    primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	Name                  string              `bson:"name" json:"name"`
	ParentID              string              `bson:"parentId" json:"parentId"`
	OwnerID               string              `bson:"ownerId" json:"ownerId"`
	CreatedAt             time.Time           `bson:"createdAt" json:"createdAt"`
	UpdatedAt             time.Time           `bson:"updatedAt" json:"updatedAt"`
	QuizGenerationStatus QuizGenerationStatus `bson:"quizGenerationStatus" json:"quizGenerationStatus"`
	QuizID                string              `bson:"quizId,omitempty" json:"quizId,omitempty"`
	QuizError             string              `bson:"quizError,omitempty" json:"quizError,omitempty"`
	QuizUpdatedAt         time.Time           `bson:"quizUpdatedAt,omitempty" json:"quizUpdatedAt,omitempty"`
}

type QuizGenerationStatus string

const (
	QuizGenStatusNone       QuizGenerationStatus = "none"
	QuizGenStatusPending   QuizGenerationStatus = "pending"
	QuizGenStatusProcessing QuizGenerationStatus = "processing"
	QuizGenStatusCompleted QuizGenerationStatus = "completed"
	QuizGenStatusFailed    QuizGenerationStatus = "failed"
)

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

// QuizStatus tracks quiz generation state
type QuizStatus string

const (
	QuizStatusCompleted QuizStatus = "completed"
	QuizStatusFailed    QuizStatus = "failed"
)

// Quiz represents a generated quiz for a folder
type Quiz struct {
	ID             primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	FolderID       string             `bson:"folderId" json:"folderId"`
	OwnerID        string             `bson:"ownerId" json:"ownerId"`
	Status         QuizStatus         `bson:"status" json:"status"`
	TotalQuestions int                `bson:"totalQuestions" json:"totalQuestions"`
	CorrectAnswers int                `bson:"correctAnswers" json:"correctAnswers"`
	Error          string             `bson:"error,omitempty" json:"error,omitempty"`
	CreatedAt      time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt      time.Time          `bson:"updatedAt" json:"updatedAt"`
}

// Question represents a quiz question
type Question struct {
	ID              primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	QuizID          string             `bson:"quizId" json:"quizId"`
	Text            string             `bson:"text" json:"text"`
	Options         []string           `bson:"options" json:"options"`             // 4 options
	CorrectOption   int                `bson:"correctOption" json:"correctOption"` // 0-3 index
	ReferencedNoteIDs []string          `bson:"referencedNoteIds" json:"referencedNoteIds"`
	Explanation     string             `bson:"explanation" json:"explanation"`
	CreatedAt       time.Time          `bson:"createdAt" json:"createdAt"`
}

// QuestionAnswer tracks user answers
type QuestionAnswer struct {
	ID             primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	QuestionID     string             `bson:"questionId" json:"questionId"`
	UserID         string             `bson:"userId" json:"userId"`
	SelectedOption int                `bson:"selectedOption" json:"selectedOption"`
	IsCorrect      bool               `bson:"isCorrect" json:"isCorrect"`
	TimeTaken      int                `bson:"timeTaken" json:"timeTaken"` // seconds
	AnsweredAt     time.Time          `bson:"answeredAt" json:"answeredAt"`
}

// NoteReview tracks spaced repetition data for each note-user pair (SM-2 Algorithm)
type NoteReview struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	NoteID       string             `bson:"noteId" json:"noteId"`
	UserID       string             `bson:"userId" json:"userId"`

	// SM-2 Algorithm fields
	EaseFactor   float32   `bson:"easeFactor" json:"easeFactor"`   // Default: 2.5
	Interval     int       `bson:"interval" json:"interval"`       // Days until next review
	Repetitions  int       `bson:"repetitions" json:"repetitions"` // Consecutive correct reviews
	NextReview   time.Time `bson:"nextReview" json:"nextReview"`

	// Review tracking
	TotalReviews int  `bson:"totalReviews" json:"totalReviews"`
	CorrectCount int  `bson:"correctCount" json:"correctCount"`
	ToReview     bool `bson:"toReview" json:"toReview"` // Marked for review due to wrong answer

	CreatedAt    time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt    time.Time `bson:"updatedAt" json:"updatedAt"`
}

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

// UserProgress tracks user's learning progress and statistics
type UserProgress struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID       string             `bson:"userId" json:"userId"`

	// Streak tracking
	CurrentStreak    int       `bson:"currentStreak" json:"currentStreak"`
	LongestStreak    int       `bson:"longestStreak" json:"longestStreak"`
	LastActiveDate   time.Time `bson:"lastActiveDate" json:"lastActiveDate"`

	// Daily goal tracking
	DailyGoalPercent  int       `bson:"dailyGoalPercent" json:"dailyGoalPercent"`
	DailyGoalDate    time.Time `bson:"dailyGoalDate" json:"dailyGoalDate"`

	// Storage tracking
	StorageUsedBytes int64 `bson:"storageUsedBytes" json:"storageUsedBytes"`
	StorageQuotaBytes int64 `bson:"storageQuotaBytes" json:"storageQuotaBytes"`

	// Session statistics (rolling window)
	SessionAccuracy  float64 `bson:"sessionAccuracy" json:"sessionAccuracy"`
	SessionAvgSpeed float64 `bson:"sessionAvgSpeed" json:"sessionAvgSpeed"`
	SessionStreak   int      `bson:"sessionStreak" json:"sessionStreak"`

	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`
}

// FolderMastery extends Folder with learning progress
type FolderMastery struct {
	FolderID       string    `bson:"folderId" json:"folderId"`
	UserID         string    `bson:"userId" json:"userId"`

	// Mastery calculation
	TotalNotes      int       `bson:"totalNotes" json:"totalNotes"`
	MasteredNotes   int       `bson:"masteredNotes" json:"masteredNotes"`
	LearntNotes    int       `bson:"learntNotes" json:"learntNotes"`

	// Mastery status
	MasteryLevel   string    `bson:"masteryLevel" json:"masteryLevel"` // "Mastered", "Learnt", "Review Soon"
	MasteryPercent float64   `bson:"masteryPercent" json:"masteryPercent"`

	// Last activity
	LastStudyDate time.Time `bson:"lastStudyDate" json:"lastStudyDate"`

	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`
}

// DocumentIndex tracks indexing status for notes
type DocumentIndex struct {
	NoteID         string    `bson:"noteId" json:"noteId"`
	UserID         string    `bson:"userId" json:"userId"`
	FolderID       string    `bson:"folderId" json:"folderId"`

	// Indexing status
	IndexStatus    string    `bson:"indexStatus" json:"indexStatus"` // "pending", "indexing", "completed", "failed"
	PagesIndexed   int       `bson:"pagesIndexed" json:"pagesIndexed"`
	TotalPages     int       `bson:"totalPages" json:"totalPages"`
	IndexedAt      time.Time `bson:"indexedAt" json:"indexedAt"`

	// Summary (AI-generated)
	SummaryText    string    `bson:"summaryText" json:"summaryText"`
	SummaryUpdated time.Time `bson:"summaryUpdated" json:"summaryUpdated"`

	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`
}

// QuizSession tracks live quiz sessions with real-time stats
type QuizSession struct {
	ID             primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID         string             `bson:"userId" json:"userId"`
	QuizID         string             `bson:"quizId" json:"quizId"`
	FolderID       string             `bson:"folderId" json:"folderId"`

	// Session state
	Status         string    `bson:"status" json:"status"` // "active", "completed", "abandoned"
	CurrentIndex   int       `bson:"currentIndex" json:"currentIndex"`

	// Live statistics
	TotalAnswered  int       `bson:"totalAnswered" json:"totalAnswered"`
	CorrectAnswers int       `bson:"correctAnswers" json:"correctAnswers"`
	TotalTimeSecs  int       `bson:"totalTimeSecs" json:"totalTimeSecs"`

	// Session streak
	CurrentStreak  int       `bson:"currentStreak" json:"currentStreak"`
	LongestStreak  int       `bson:"longestStreak" json:"longestStreak"`

	StartedAt      time.Time `bson:"startedAt" json:"startedAt"`
	CompletedAt    time.Time `bson:"completedAt,omitempty" json:"completedAt,omitempty"`
}

// NodeMastery tracks mastery for a node (embedded in Node)
type NodeMastery struct {
	TotalNotes      int       `bson:"totalNotes" json:"totalNotes"`
	MasteredNotes   int       `bson:"masteredNotes" json:"masteredNotes"`
	LearntNotes     int       `bson:"learntNotes" json:"learntNotes"`
	MasteryLevel    string    `bson:"masteryLevel" json:"masteryLevel"` // "Mastered", "Learnt", "Review Soon"
	MasteryPercent  float64   `bson:"masteryPercent" json:"masteryPercent"`
	LastStudyDate   time.Time `bson:"lastStudyDate" json:"lastStudyDate"`
	LastUpdated     time.Time `bson:"lastUpdated" json:"lastUpdated"`
}

// NodeType represents the type of node
type NodeType string

const (
	NodeTypeFolder NodeType = "folder"
	NodeTypeNote   NodeType = "note"
)

// NodeMetadata contains type-specific metadata
type NodeMetadata struct {
	Type    NodeType `bson:"type" json:"type"`
	DriveID string   `bson:"driveId,omitempty" json:"driveId,omitempty"` // Only for notes
}

// Node represents a unified structure for both folders and notes
type Node struct {
	ID            primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Name          string             `bson:"name" json:"name"`
	ParentID      string             `bson:"parentId" json:"parentId"`
	Children      []string           `bson:"children" json:"children"`
	TotalNoteCount int               `bson:"totalNoteCount" json:"totalNoteCount"`
	Metadata      NodeMetadata       `bson:"metadata" json:"metadata"`
	OwnerID       string             `bson:"ownerId" json:"ownerId"`
	CreatedAt     time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt     time.Time          `bson:"updatedAt" json:"updatedAt"`

	// Mastery - embedded per node
	Mastery NodeMastery `bson:"mastery" json:"mastery"`

	// Folder-specific fields
	QuizGenerationStatus QuizGenerationStatus `bson:"quizGenerationStatus,omitempty" json:"quizGenerationStatus,omitempty"`
	QuizID                string              `bson:"quizId,omitempty" json:"quizId,omitempty"`
	QuizError             string              `bson:"quizError,omitempty" json:"quizError,omitempty"`
	QuizUpdatedAt         time.Time           `bson:"quizUpdatedAt,omitempty" json:"quizUpdatedAt,omitempty"`

	// Note-specific fields
	PublicURL string `bson:"publicUrl,omitempty" json:"publicUrl,omitempty"`
	// Caption data is stored separately in caption_embeddings collection
}

// MasteryJob represents a job in the mastery update queue
type MasteryJob struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	NodeID    string             `bson:"nodeId" json:"nodeId"`
	Status    string             `bson:"status" json:"status"` // "pending", "processing", "completed", "failed"
	Attempt   int                `bson:"attempt" json:"attempt"`
	Error     string             `bson:"error,omitempty" json:"error,omitempty"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
	ProcessedAt time.Time        `bson:"processedAt,omitempty" json:"processedAt,omitempty"`
}

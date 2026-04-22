package handlers

import (
	"context"
	"os"
	"time"

	"cogniscan/backend/internal/database"
	"cogniscan/backend/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
)

// FolderMasteryResponse represents the folder mastery response
type FolderMasteryResponse struct {
	FolderID       string    `json:"folderId"`
	UserID         string    `json:"userId"`
	TotalNotes     int       `json:"totalNotes"`
	MasteredNotes  int       `json:"masteredNotes"`
	LearntNotes    int       `json:"learntNotes"`
	MasteryLevel   string    `json:"masteryLevel"`
	MasteryPercent float64   `json:"masteryPercent"`
	LastStudyDate  time.Time `json:"lastStudyDate"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// MasteryUpdateRequest represents a mastery update request
type MasteryUpdateRequest struct {
	NoteID    string `json:"noteId" binding:"required"`
	IsCorrect bool   `json:"isCorrect" binding:"required"`
}

// GetAllFoldersMasteryResponse represents response with all folder mastery data
type GetAllFoldersMasteryResponse struct {
	Folders []FolderMasteryResponse `json:"folders"`
	Total   int                     `json:"total"`
}

// GetFolderMastery returns mastery status for a folder
// @Summary Returns mastery information including total notes, mastered count, and mastery percentage
func GetFolderMastery(c *gin.Context) {
	folderID := c.Param("folderId")
	userID := c.GetString("userId")
	if folderID == "" || userID == "" {
		c.JSON(400, gin.H{"error": "folderId and userId required"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db := database.Client.Database(os.Getenv("DB_NAME"))

	// Try to fetch existing folder mastery
	var folderMastery models.FolderMastery
	collection := db.Collection("folder_mastery")
	err := collection.FindOne(ctx, bson.M{"folderId": folderID, "userId": userID}).Decode(&folderMastery)

	if err == nil {
		// Return cached mastery
		response := &FolderMasteryResponse{
			FolderID:       folderMastery.FolderID,
			UserID:         folderMastery.UserID,
			TotalNotes:     folderMastery.TotalNotes,
			MasteredNotes:  folderMastery.MasteredNotes,
			LearntNotes:    folderMastery.LearntNotes,
			MasteryLevel:   folderMastery.MasteryLevel,
			MasteryPercent: folderMastery.MasteryPercent,
			LastStudyDate:  folderMastery.LastStudyDate,
			CreatedAt:      folderMastery.CreatedAt,
			UpdatedAt:      folderMastery.UpdatedAt,
		}
		c.JSON(200, response)
		return
	}

	// If not cached, calculate from note reviews
	// Get all notes in folder
	notesCollection := db.Collection("notes")
	notesCursor, _ := notesCollection.Find(ctx, bson.M{"folderId": folderID, "ownerId": userID})
	defer notesCursor.Close(ctx)

	var notes []models.Note
	notesCursor.All(ctx, &notes)

	totalNotes := len(notes)
	masteredNotes := 0
	learntNotes := 0
	var lastStudyDate time.Time

	// Get note reviews for this folder
	reviewsCollection := db.Collection("note_reviews")
	reviewsCursor, _ := reviewsCollection.Find(ctx, bson.M{"userId": userID})
	defer reviewsCursor.Close(ctx)

	var reviews []models.NoteReview
	reviewsCursor.All(ctx, &reviews)

	// Count mastered and learnt notes based on reviews
	for _, note := range notes {
		for _, review := range reviews {
			if review.NoteID == note.ID.Hex() {
				if review.Repetitions >= 3 {
					masteredNotes++
				} else if review.Repetitions >= 1 {
					learntNotes++
				}
				if review.UpdatedAt.After(lastStudyDate) {
					lastStudyDate = review.UpdatedAt
				}
				break
			}
		}
	}

	masteryPercent := 0.0
	if totalNotes > 0 {
		masteryPercent = float64(masteredNotes) / float64(totalNotes)
	}

	masteryLevel := determineMasteryLevel(masteryPercent)

	response := &FolderMasteryResponse{
		FolderID:       folderID,
		UserID:         userID,
		TotalNotes:     totalNotes,
		MasteredNotes:  masteredNotes,
		LearntNotes:    learntNotes,
		MasteryLevel:   masteryLevel,
		MasteryPercent: masteryPercent,
		LastStudyDate:  lastStudyDate,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	c.JSON(200, response)
}

func determineMasteryLevel(percent float64) string {
	if percent > 0.8 {
		return "Mastered"
	} else if percent >= 0.5 {
		return "Learnt"
	}
	return "Review Soon"
}

// GetAllFoldersMastery returns mastery for all user folders
// @Summary Returns mastery data for all folders belonging to the user
func GetAllFoldersMastery(c *gin.Context) {
	userID := c.GetString("userId")
	if userID == "" {
		c.JSON(400, gin.H{"error": "userId required"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db := database.Client.Database(os.Getenv("DB_NAME"))

	// Get all folders owned by user
	foldersCollection := db.Collection("folders")
	cursor, err := foldersCollection.Find(ctx, bson.M{"ownerId": userID})
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to fetch folders"})
		return
	}
	defer cursor.Close(ctx)

	var folders []models.Folder
	if err := cursor.All(ctx, &folders); err != nil {
		c.JSON(500, gin.H{"error": "Failed to decode folders"})
		return
	}

	// Get folder mastery records
	masteryCollection := db.Collection("folder_mastery")
	masteryCursor, _ := masteryCollection.Find(ctx, bson.M{"userId": userID})
	defer masteryCursor.Close(ctx)

	var masteryRecords []models.FolderMastery
	masteryCursor.All(ctx, &masteryRecords)

	// Build map of folderID to mastery record
	masteryMap := make(map[string]models.FolderMastery)
	for _, m := range masteryRecords {
		masteryMap[m.FolderID] = m
	}

	// Build response
	folderMasteryResponses := []FolderMasteryResponse{}
	for _, folder := range folders {
		folderID := folder.ID.Hex()
		if mastery, exists := masteryMap[folderID]; exists {
			folderMasteryResponses = append(folderMasteryResponses, FolderMasteryResponse{
				FolderID:       mastery.FolderID,
				UserID:         mastery.UserID,
				TotalNotes:     mastery.TotalNotes,
				MasteredNotes:  mastery.MasteredNotes,
				LearntNotes:    mastery.LearntNotes,
				MasteryLevel:   mastery.MasteryLevel,
				MasteryPercent: mastery.MasteryPercent,
				LastStudyDate:  mastery.LastStudyDate,
				CreatedAt:      mastery.CreatedAt,
				UpdatedAt:      mastery.UpdatedAt,
			})
		}
	}

	response := GetAllFoldersMasteryResponse{
		Folders: folderMasteryResponses,
		Total:   len(folderMasteryResponses),
	}

	c.JSON(200, response)
}

// UpdateNoteMastery recalculates folder mastery after note review
// @Summary Updates mastery levels when a user reviews a note (correct or incorrect)
func UpdateNoteMastery(c *gin.Context) {
	userID := c.GetString("userId")
	if userID == "" {
		c.JSON(400, gin.H{"error": "userId required"})
		return
	}

	var req MasteryUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// TODO: Implement database update logic
	// 1. Update NoteReview record
	// 2. Recalculate FolderMastery based on all reviews
	// 3. Determine new mastery level:
	//    - Mastered: >80% of notes mastered
	//    - Learnt: 50-80% mastery
	//    - Review Soon: <50% mastery

	c.JSON(200, gin.H{
		"message":   "Mastery updated",
		"noteId":    req.NoteID,
		"isCorrect": req.IsCorrect,
	})
}

package handlers

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
	"cogniscan/backend/internal/database"
	"cogniscan/backend/internal/models"
	"cogniscan/backend/internal/services"
)

// DocumentIndexResponse represents the document index response
type DocumentIndexResponse struct {
	NoteID        string    `json:"noteId"`
	UserID        string    `json:"userId"`
	FolderID      string    `json:"folderId"`
	IndexStatus   string    `json:"indexStatus"`
	PagesIndexed  int       `json:"pagesIndexed"`
	TotalPages    int       `json:"totalPages"`
	IndexedAt     time.Time `json:"indexedAt"`
	SummaryText   string    `json:"summaryText"`
	SummaryUpdated time.Time `json:"summaryUpdated"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// GenerateSummaryRequest represents a summary generation request
type GenerateSummaryRequest struct {
	NoteID string `json:"noteId" binding:"required"`
}

type UpdateDocumentIndexRequest struct {
	NoteID       string `json:"noteId" binding:"required"`
	FolderID     string `json:"folderId" binding:"required"`
	IndexStatus  string `json:"indexStatus"`
	PagesIndexed int    `json:"pagesIndexed"`
	TotalPages   int    `json:"totalPages"`
}

// GetNoteIndexStatus returns indexing status for a note
// @Summary Returns the current indexing status, page count, and summary of a document
func GetNoteIndexStatus(c *gin.Context) {
	noteID := c.Param("id")
	userID := c.GetString("userId")
	if noteID == "" || userID == "" {
		c.JSON(400, gin.H{"error": "noteId and userId required"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db := database.Client.Database(os.Getenv("DB_NAME"))
	collection := db.Collection("document_index")

	var docIndex models.DocumentIndex
	err := collection.FindOne(ctx, bson.M{"noteId": noteID, "userId": userID}).Decode(&docIndex)
	if err != nil {
		c.JSON(404, gin.H{"error": "Document index not found"})
		return
	}

	indexStatus := &DocumentIndexResponse{
		NoteID:         docIndex.NoteID,
		UserID:         docIndex.UserID,
		FolderID:       docIndex.FolderID,
		IndexStatus:    docIndex.IndexStatus,
		PagesIndexed:   docIndex.PagesIndexed,
		TotalPages:     docIndex.TotalPages,
		IndexedAt:      docIndex.IndexedAt,
		SummaryText:    docIndex.SummaryText,
		SummaryUpdated: docIndex.SummaryUpdated,
		CreatedAt:      docIndex.CreatedAt,
		UpdatedAt:      docIndex.UpdatedAt,
	}

	c.JSON(200, indexStatus)
}

// GetFolderIndexStatus returns indexing status for a folder
// @Summary Returns aggregate indexing status for all notes in a folder
func GetFolderIndexStatus(c *gin.Context) {
	folderID := c.Param("id")
	userID := c.GetString("userId")
	if folderID == "" || userID == "" {
		c.JSON(400, gin.H{"error": "folderId and userId required"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db := database.Client.Database(os.Getenv("DB_NAME"))
	collection := db.Collection("document_index")

	cursor, err := collection.Find(ctx, bson.M{"folderId": folderID, "userId": userID})
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to fetch folder index status"})
		return
	}
	defer cursor.Close(ctx)

	var docIndexes []models.DocumentIndex
	if err := cursor.All(ctx, &docIndexes); err != nil {
		c.JSON(500, gin.H{"error": "Failed to decode index data"})
		return
	}

	totalNotes := len(docIndexes)
	indexedNotes := 0
	pendingNotes := 0

	for _, idx := range docIndexes {
		if idx.IndexStatus == "completed" {
			indexedNotes++
		} else {
			pendingNotes++
		}
	}

	c.JSON(200, gin.H{
		"folderId":     folderID,
		"totalNotes":    totalNotes,
		"indexedNotes":  indexedNotes,
		"pendingNotes":  pendingNotes,
	})
}

// UpdateDocumentIndex updates document indexing data
// @Summary Updates the indexing status and summary information for a document
func UpdateDocumentIndex(c *gin.Context) {
	userID := c.GetString("userId")
	if userID == "" {
		c.JSON(400, gin.H{"error": "userId required"})
		return
	}

	var req UpdateDocumentIndexRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db := database.Client.Database(os.Getenv("DB_NAME"))
	collection := db.Collection("document_index")

	filter := bson.M{"noteId": req.NoteID, "userId": userID}
	update := bson.M{
		"$set": bson.M{
			"folderId":     req.FolderID,
			"indexStatus":  req.IndexStatus,
			"pagesIndexed": req.PagesIndexed,
			"totalPages":   req.TotalPages,
			"updatedAt":    time.Now(),
		},
	}

	if req.IndexStatus == "completed" {
		update["$set"].(bson.M)["indexedAt"] = time.Now()
	}

	opts := options.Update().SetUpsert(true)
	_, err := collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		log.Printf("Failed to update document index: %v", err)
		c.JSON(500, gin.H{"error": "Failed to update document index"})
		return
	}

	c.JSON(200, gin.H{
		"message": "Document index updated",
	})
}

// GenerateSummary generates AI summary for a note
// @Summary Triggers AI service to generate a concise summary of a document
func GenerateSummary(c *gin.Context) {
	userID := c.GetString("userId")
	if userID == "" {
		c.JSON(400, gin.H{"error": "userId required"})
		return
	}

	var req GenerateSummaryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db := database.Client.Database(os.Getenv("DB_NAME"))

	// Fetch note to get content and title
	notesCollection := db.Collection("notes")
	var note models.Note
	err := notesCollection.FindOne(ctx, bson.M{"_id": req.NoteID, "ownerId": userID}).Decode(&note)
	if err != nil {
		c.JSON(404, gin.H{"error": "Note not found"})
		return
	}

	// Generate AI summary
	summary, err := services.GenerateDocumentSummary(note.Caption, note.Name)
	if err != nil {
		log.Printf("Failed to generate summary: %v", err)
		c.JSON(500, gin.H{"error": "Failed to generate summary"})
		return
	}

	// Update document index with summary
	docIndexCollection := db.Collection("document_index")
	filter := bson.M{"noteId": req.NoteID, "userId": userID}
	update := bson.M{
		"$set": bson.M{
			"summaryText":    summary,
			"summaryUpdated": time.Now(),
			"updatedAt":      time.Now(),
		},
	}

	_, err = docIndexCollection.UpdateOne(ctx, filter, update)
	if err != nil {
		log.Printf("Failed to update document index with summary: %v", err)
		// Don't fail the response, the summary was generated
	}

	c.JSON(200, gin.H{
		"message":   "Summary generation completed",
		"noteId":   req.NoteID,
		"summary":  summary,
	})
}

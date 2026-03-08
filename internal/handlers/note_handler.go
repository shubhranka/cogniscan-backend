// ./cogniscan-backend/internal/handlers/note_handler.go
package handlers

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"cogniscan/backend/internal/database"
	"cogniscan/backend/internal/middleware"
	"cogniscan/backend/internal/models"
	"cogniscan/backend/internal/queue"
	"cogniscan/backend/internal/services"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// CreateNote is correct and unchanged from the previous phase.
func CreateNote(c *gin.Context) {
	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	err := c.Request.ParseMultipartForm(20 << 20)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Error parsing form data"})
		return
	}
	name := c.Request.FormValue("name")
	folderId := c.Request.FormValue("folderId")
	file, header, err := c.Request.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Image file is required"})
		return
	}
	defer file.Close()

	// Read image bytes for AI processing
	imageBytes, err := io.ReadAll(file)
	if err != nil {
		log.Printf("[NoteHandler] Failed to read image bytes: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process image"})
		return
	}

	driveID, err := services.UploadFile(header.Filename, bytes.NewReader(imageBytes))
	if err != nil {
		log.Printf("[NoteHandler] Failed to upload to Google Drive: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload file"})
		return
	}

	now := time.Now()
	noteID := primitive.NewObjectID()
	newNote := models.Note{
		ID:            noteID,
		Name:          name,
		PublicURL:     "", // REFACTORED
		DriveID:       driveID,
		Caption:       "", // Initially empty, will be generated asynchronously
		CaptionStatus: models.CaptionStatusPending,
		CreatedAt:     now,
		UpdatedAt:     now,
		FolderID:      folderId,
		OwnerID:       firebaseUser.UID,
	}
	notesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("notes")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = notesCollection.InsertOne(ctx, newNote)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save note record"})
		return
	}

	// Enqueue caption generation job for background processing
	// Worker will download image from Drive, generate caption, and update MongoDB
	job := queue.CaptionJob{
		ID:      primitive.NewObjectID().Hex(),
		NoteID:  noteID.Hex(),
		DriveID: driveID,
	}
	if err := services.EnqueueCaptionJob(job); err != nil {
		log.Printf("[NoteHandler] Failed to enqueue caption job for note %s: %v", noteID.Hex(), err)
		// Note is saved with "pending" status, can be retried later
	} else {
		log.Printf("[NoteHandler] Enqueued caption job %s for note %s", job.ID, noteID.Hex())
	}

	c.JSON(http.StatusCreated, newNote)
}

// NEW HANDLER to serve as the secure proxy
func GetNoteImage(c *gin.Context) {
	noteID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid note ID"})
		return
	}
	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// 1. Fetch the Note from MongoDB to get the DriveID and verify ownership
	notesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("notes")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var note models.Note
	filter := bson.M{"_id": noteID, "ownerId": firebaseUser.UID}
	if err := notesCollection.FindOne(ctx, filter).Decode(&note); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Note not found or you don't have permission"})
		return
	}

	if note.DriveID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "No image associated with this note"})
		return
	}

	// 2. Download from Google Drive server-side
	log.Printf("[GetNoteImage] Downloading DriveID: %s", note.DriveID)
	resp, err := services.DownloadFileContent(note.DriveID)
	if err != nil {
		log.Printf("[GetNoteImage] Error downloading from Drive: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not retrieve image from storage"})
		return
	}
	defer resp.Body.Close()

	// 3. Stream the file back to the client
	c.Header("Content-Type", resp.Header.Get("Content-Type"))
	c.Header("Content-Length", resp.Header.Get("Content-Length"))

	// Stream the body
	_, err = io.Copy(c.Writer, resp.Body)
	if err != nil {
		log.Printf("[GetNoteImage] Error streaming file to client: %v", err)
	}
}

// GetNotesInFolder is correct and unchanged from the previous phase.
func GetNotesInFolder(c *gin.Context) {
	parentID := c.Param("folderId")

	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	notesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("notes")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	filter := bson.M{
		"ownerId":  firebaseUser.UID,
		"folderId": parentID,
	}

	cursor, err := notesCollection.Find(ctx, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch notes"})
		return
	}
	defer cursor.Close(ctx)

	var notes []models.Note
	if err = cursor.All(ctx, &notes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode notes"})
		return
	}

	if notes == nil {
		notes = make([]models.Note, 0)
	}

	c.JSON(http.StatusOK, notes)
}

// --- NEW/UPDATED HANDLERS ---

// UpdateNotePayload defines the expected JSON for renaming a note
type UpdateNotePayload struct {
	Name string `json:"name" binding:"required"`
}

// UpdateNote renames a note.
func UpdateNote(c *gin.Context) {
	noteID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid note ID"})
		return
	}

	var payload UpdateNotePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
		return
	}

	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	notesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("notes")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{"_id": noteID, "ownerId": firebaseUser.UID}
	update := bson.M{"$set": bson.M{"name": payload.Name}}

	result, err := notesCollection.UpdateOne(ctx, filter, update)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update note"})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Note not found or you don't have permission"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Note updated successfully"})
}

// DeleteNote now deletes from Google Drive.
func DeleteNote(c *gin.Context) {
	noteID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid note ID"})
		return
	}
	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	notesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("notes")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find the note to get its DriveID
	var noteToDelete models.Note
	filter := bson.M{"_id": noteID, "ownerId": firebaseUser.UID}
	err = notesCollection.FindOne(ctx, filter).Decode(&noteToDelete)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Note not found"})
		return
	}

	// Delete from Google Drive
	if noteToDelete.DriveID != "" {
		err := services.DeleteFile(noteToDelete.DriveID)
		if err != nil {
			// Log the error but continue, as the main goal is to remove it from the app
			log.Printf("Could not delete file from Google Drive: %v", err)
		}
	}

	// Delete from MongoDB
	_, err = notesCollection.DeleteOne(ctx, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete note from database"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Note deleted successfully"})
}

// RegenerateCaption regenerates the caption for an existing note
func RegenerateCaption(c *gin.Context) {
	noteID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid note ID"})
		return
	}
	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Fetch the note to verify ownership and get DriveID
	notesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("notes")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var note models.Note
	filter := bson.M{"_id": noteID, "ownerId": firebaseUser.UID}
	if err := notesCollection.FindOne(ctx, filter).Decode(&note); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Note not found or you don't have permission"})
		return
	}

	if note.DriveID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No image associated with this note"})
		return
	}

	// Update note status to "pending" and enqueue caption generation job
	job := queue.CaptionJob{
		ID:      primitive.NewObjectID().Hex(),
		NoteID:  noteID.Hex(),
		DriveID: note.DriveID,
	}

	// First update status to "pending" in MongoDB
	update := bson.M{"$set": bson.M{"captionStatus": models.CaptionStatusPending, "captionError": "", "updatedAt": time.Now()}}
	if _, err := notesCollection.UpdateOne(ctx, filter, update); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update note status"})
		return
	}

	// Enqueue job for background processing
	if err := services.EnqueueCaptionJob(job); err != nil {
		log.Printf("[RegenerateCaption] Failed to enqueue caption job for note %s: %v", noteID.Hex(), err)
		// Note status is "pending", can be retried later
	}

	log.Printf("[RegenerateCaption] Enqueued caption job %s for note %s", job.ID, noteID.Hex())
	c.JSON(http.StatusAccepted, gin.H{
		"message": "Caption regeneration started",
		"noteId":  noteID.Hex(),
	})
}

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
		ID:        noteID,
		Name:      name,
		PublicURL: "", // REFACTORED
		DriveID:   driveID,
		Caption:   "", // Initially empty, will be generated asynchronously
		CreatedAt: now,
		UpdatedAt: now,
		FolderID:  folderId,
		OwnerID:   firebaseUser.UID,
	}
	notesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("notes")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = notesCollection.InsertOne(ctx, newNote)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save note record"})
		return
	}

	// Spawn background goroutine for async caption generation
	// This doesn't block the response - caption will be populated later
	go func() {
		caption, err := services.GenerateCaption(imageBytes)
		if err != nil {
			log.Printf("[NoteHandler] Caption generation failed for note %s: %v", noteID.Hex(), err)
			// Graceful degradation - note remains without caption
			return
		}

		if err := services.UpdateNoteCaption(noteID.Hex(), caption); err != nil {
			log.Printf("[NoteHandler] Failed to update caption for note %s: %v", noteID.Hex(), err)
		} else {
			log.Printf("[NoteHandler] Caption generated and saved for note %s", noteID.Hex())
		}
	}()

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

	// Download image from Drive
	resp, err := services.DownloadFileContent(note.DriveID)
	if err != nil {
		log.Printf("[RegenerateCaption] Failed to download image from Drive: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve image"})
		return
	}
	defer resp.Body.Close()

	imageBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[RegenerateCaption] Failed to read image bytes: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process image"})
		return
	}

	// Spawn goroutine for async caption generation
	go func() {
		caption, err := services.GenerateCaption(imageBytes)
		if err != nil {
			log.Printf("[RegenerateCaption] Caption generation failed for note %s: %v", noteID.Hex(), err)
			return
		}

		if err := services.UpdateNoteCaption(noteID.Hex(), caption); err != nil {
			log.Printf("[RegenerateCaption] Failed to update caption: %v", err)
		} else {
			log.Printf("[RegenerateCaption] Caption regenerated for note %s", noteID.Hex())
		}
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"message": "Caption regeneration started",
		"noteId":  noteID.Hex(),
	})
}

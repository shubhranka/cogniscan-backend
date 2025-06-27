// ./cogniscan-backend/internal/handlers/note_handler.go
package handlers

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cogniscan/backend/internal/database"
	"cogniscan/backend/internal/middleware"
	"cogniscan/backend/internal/models"
	"cogniscan/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/u3mur4/megadl"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// CreateNote is correct and unchanged from the previous phase.
func CreateNote(c *gin.Context) {
	log.Println("--- [CreateNote Handler] Received a request ---")
	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	err := c.Request.ParseMultipartForm(20 << 20) // 20MB max
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

	tempFile, err := os.CreateTemp("", "upload-*.tmp")
	if err != nil {
		log.Printf("[CreateNote Handler] FATAL ERROR: Could not create temp file: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Server error during file processing"})
		return
	}
	defer tempFile.Close()
	defer os.Remove(tempFile.Name())

	_, err = io.Copy(tempFile, file)
	if err != nil {
		log.Printf("[CreateNote Handler] FATAL ERROR: Could not copy to temp file: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Server error during file saving"})
		return
	}

	megaSvc := services.GetClient()
	if megaSvc == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Storage service not available"})
		return
	}

	cogniScanNode, err := services.FindOrCreateCogniScanNode(megaSvc)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not access storage folder"})
		return
	}

	node, err := megaSvc.UploadFile(tempFile.Name(), cogniScanNode, header.Filename, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "File upload failed"})
		log.Printf("[CreateNote Handler] FATAL ERROR: File upload failed: %v", err)
		return
	}

	link, err := megaSvc.Link(node, true)
	fmt.Println(link)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate file link"})
		return
	}

	newNote := models.Note{
		ID:        primitive.NewObjectID(),
		Name:      name,
		MegaURL:   link,
		CreatedAt: time.Now(),
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
	c.JSON(http.StatusCreated, newNote)
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

	// 1. Fetch the Note from MongoDB to get the MegaURL
	notesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("notes")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var note models.Note
	filter := bson.M{"_id": noteID, "ownerId": firebaseUser.UID}
	if err := notesCollection.FindOne(ctx, filter).Decode(&note); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Note not found or you don't have permission"})
		return
	}

	if note.MegaURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No image URL associated with this note"})
		return
	}

	// The u3mur4/megadl library expects the 'mega.nz' domain, so we replace the old 'mega.co.nz' domain.
	downloadURL := strings.Replace(note.MegaURL, "mega.co.nz", "mega.nz", 1)

	// 2. Download from MEGA public URL
	log.Printf("[GetNoteImage Handler] Attempting to download from MEGA URL: %s", downloadURL)
	reader, _, err := megadl.Download(downloadURL)
	if err != nil {
		log.Printf("[GetNoteImage Handler] FATAL ERROR: Could not download from MEGA link %s: %v", downloadURL, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or expired storage link"})
		return
	}
	defer reader.Close()

	// 3. Stream the file back to the client
	ext := filepath.Ext(note.Name)
	contentType := "application/octet-stream" // Default
	if ext == ".jpg" || ext == ".jpeg" {
		contentType = "image/jpeg"
	} else if ext == ".png" {
		contentType = "image/png"
	} else if ext == ".webp" {
		contentType = "image/webp"
	}

	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", note.Name))
	c.Header("Content-Type", contentType)

	_, err = io.Copy(c.Writer, reader)
	if err != nil {
		log.Printf("[GetNoteImage Handler] FATAL ERROR: Failed to stream file to client: %v", err)
	}
}

// DeleteNote deletes a single note from MongoDB. The MEGA file is orphaned.
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

	log.Printf("Deleting note %s from database. The corresponding MEGA file will be orphaned.", noteID.Hex())

	// Delete from MongoDB
	filter := bson.M{"_id": noteID, "ownerId": firebaseUser.UID}
	result, err := notesCollection.DeleteOne(ctx, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete note from database"})
		return
	}

	if result.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Note not found or you don't have permission"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Note deleted successfully from database."})
}

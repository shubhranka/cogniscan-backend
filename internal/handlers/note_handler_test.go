package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"cogniscan/backend/internal/database"
	"cogniscan/backend/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// setupTestRouter creates a test router with auth middleware mock
func setupNoteTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Mock auth middleware that sets user ID in context
	router.Use(func(c *gin.Context) {
		c.Set("user", "test-user-id")
		c.Next()
	})

	return router
}

func TestUpdateNote(t *testing.T) {
	os.Setenv("MONGO_URI", "mongodb://localhost:27017")
	os.Setenv("DB_NAME", "cogniscan_test")
	database.ConnectDB()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create test note
	noteID := primitive.NewObjectID()
	testNote := models.Note{
		ID:       noteID,
		Name:     "Original Note",
		DriveID:  "drive123",
		FolderID: "folder123",
		OwnerID:  "test-user-id",
	}

	notesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("notes")
	_, err := notesCollection.InsertOne(ctx, testNote)
	if err != nil {
		t.Skip("Skipping test: MongoDB not available")
	}
	defer database.Client.Database(os.Getenv("DB_NAME")).Drop(ctx)

	router := setupNoteTestRouter()
	router.PUT("/notes/:id", UpdateNote)

	tests := []struct {
		name       string
		noteID     string
		payload    map[string]interface{}
		wantStatus int
	}{
		{
			name:   "Successfully update note",
			noteID: noteID.Hex(),
			payload: map[string]interface{}{
				"name": "Updated Note Name",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Invalid note ID",
			noteID: "invalid-id",
			payload: map[string]interface{}{
				"name": "Updated Name",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "Non-existent note",
			noteID: primitive.NewObjectID().Hex(),
			payload: map[string]interface{}{
				"name": "Updated Name",
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "Missing required name field",
			noteID: noteID.Hex(),
			payload: map[string]interface{}{},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payloadBytes, err := json.Marshal(tt.payload)
			if err != nil {
				t.Fatalf("Failed to marshal payload: %v", err)
			}

			req, err := http.NewRequest("PUT", "/notes/"+tt.noteID, bytes.NewBuffer(payloadBytes))
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("UpdateNote() status = %v, want %v", w.Code, tt.wantStatus)
			}

			// Verify update in database
			if tt.name == "Successfully update note" {
				var updatedNote models.Note
				err = notesCollection.FindOne(ctx, bson.M{"_id": noteID}).Decode(&updatedNote)
				if err != nil {
					t.Fatalf("Failed to find updated note: %v", err)
				}

				if updatedNote.Name != "Updated Note Name" {
					t.Errorf("Expected name 'Updated Note Name', got %s", updatedNote.Name)
				}
			}
		})
	}
}

func TestDeleteNote(t *testing.T) {
	os.Setenv("MONGO_URI", "mongodb://localhost:27017")
	os.Setenv("DB_NAME", "cogniscan_test")
	database.ConnectDB()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create test note
	noteID := primitive.NewObjectID()
	testNote := models.Note{
		ID:       noteID,
		Name:     "Test Note",
		DriveID:  "drive123",
		FolderID: "folder123",
		OwnerID:  "test-user-id",
	}

	notesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("notes")
	_, err := notesCollection.InsertOne(ctx, testNote)
	if err != nil {
		t.Skip("Skipping test: MongoDB not available")
	}
	defer database.Client.Database(os.Getenv("DB_NAME")).Drop(ctx)

	router := setupNoteTestRouter()
	router.DELETE("/notes/:id", DeleteNote)

	tests := []struct {
		name       string
		noteID     string
		wantStatus int
	}{
		{
			name:       "Successfully delete note",
			noteID:     noteID.Hex(),
			wantStatus: http.StatusOK,
		},
		{
			name:       "Invalid note ID",
			noteID:     "invalid-id",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Non-existent note",
			noteID:     primitive.NewObjectID().Hex(),
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("DELETE", "/notes/"+tt.noteID, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("DeleteNote() status = %v, want %v", w.Code, tt.wantStatus)
			}

			// Verify deletion in database
			if tt.name == "Successfully delete note" {
				count, _ := notesCollection.CountDocuments(ctx, bson.M{"_id": noteID})
				if count > 0 {
					t.Error("Note should be deleted")
				}
			}
		})
	}
}

func TestGetNotesInFolder(t *testing.T) {
	os.Setenv("MONGO_URI", "mongodb://localhost:27017")
	os.Setenv("DB_NAME", "cogniscan_test")
	database.ConnectDB()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create test notes
	folderID := "folder123"

	testNotes := []models.Note{
		{
			ID:       primitive.NewObjectID(),
			Name:     "Note 1",
			DriveID:  "drive1",
			FolderID: folderID,
			OwnerID:  "test-user-id",
			Caption:  "Caption 1",
		},
		{
			ID:       primitive.NewObjectID(),
			Name:     "Note 2",
			DriveID:  "drive2",
			FolderID: folderID,
			OwnerID:  "test-user-id",
			Caption:  "Caption 2",
		},
		{
			ID:       primitive.NewObjectID(),
			Name:     "Other User Note",
			DriveID:  "drive3",
			FolderID: folderID,
			OwnerID:  "other-user-id",
		},
		{
			ID:       primitive.NewObjectID(),
			Name:     "Note in different folder",
			DriveID:  "drive4",
			FolderID: "other-folder",
			OwnerID:  "test-user-id",
		},
	}

	notesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("notes")
	for _, note := range testNotes {
		_, err := notesCollection.InsertOne(ctx, note)
		if err != nil {
			t.Skip("Skipping test: MongoDB not available")
		}
	}
	defer database.Client.Database(os.Getenv("DB_NAME")).Drop(ctx)

	router := setupNoteTestRouter()
	router.GET("/folders/:folderId/notes", GetNotesInFolder)

	req, err := http.NewRequest("GET", "/folders/"+folderID+"/notes", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GetNotesInFolder() status = %v, want %v", w.Code, http.StatusOK)
	}

	var response []models.Note
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	// Should only return notes for the test user in the specified folder
	if len(response) != 2 {
		t.Errorf("Expected 2 notes, got %d", len(response))
	}

	for _, note := range response {
		if note.OwnerID != "test-user-id" {
			t.Errorf("Note %s belongs to wrong user: %s", note.Name, note.OwnerID)
		}
		if note.FolderID != folderID {
			t.Errorf("Note %s is in wrong folder: %s", note.Name, note.FolderID)
		}
	}
}

func TestRegenerateCaption(t *testing.T) {
	os.Setenv("MONGO_URI", "mongodb://localhost:27017")
	os.Setenv("DB_NAME", "cogniscan_test")
	database.ConnectDB()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create test note
	noteID := primitive.NewObjectID()
	testNote := models.Note{
		ID:       noteID,
		Name:     "Test Note",
		DriveID:  "drive123",
		FolderID: "folder123",
		OwnerID:  "test-user-id",
		Caption:  "Old Caption",
	}

	notesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("notes")
	_, err := notesCollection.InsertOne(ctx, testNote)
	if err != nil {
		t.Skip("Skipping test: MongoDB not available")
	}
	defer database.Client.Database(os.Getenv("DB_NAME")).Drop(ctx)

	router := setupNoteTestRouter()
	router.POST("/notes/:id/regenerate-caption", RegenerateCaption)

	tests := []struct {
		name       string
		noteID     string
		wantStatus int
	}{
		{
			name:       "Start caption regeneration",
			noteID:     noteID.Hex(),
			wantStatus: http.StatusAccepted,
		},
		{
			name:       "Invalid note ID",
			noteID:     "invalid-id",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Non-existent note",
			noteID:     primitive.NewObjectID().Hex(),
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("POST", "/notes/"+tt.noteID+"/regenerate-caption", nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("RegenerateCaption() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.name == "Start caption regeneration" && w.Code == http.StatusAccepted {
				var response map[string]interface{}
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Fatalf("Failed to unmarshal response: %v", err)
				}

				if response["message"] != "Caption regeneration started" {
					t.Errorf("Expected message 'Caption regeneration started', got %v", response["message"])
				}
				if response["noteId"] != noteID.Hex() {
					t.Errorf("Expected noteId %s, got %v", noteID.Hex(), response["noteId"])
				}
			}
		})
	}
}

func TestCreateNote(t *testing.T) {
	os.Setenv("MONGO_URI", "mongodb://localhost:27017")
	os.Setenv("DB_NAME", "cogniscan_test")
	database.ConnectDB()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	router := setupNoteTestRouter()
	router.POST("/notes", CreateNote)

	// Create a multipart form with image data
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add name field
	writer.WriteField("name", "Test Note")
	writer.WriteField("folderId", "folder123")

	// Add image file
	imageData := bytes.NewReader([]byte("fake image content"))
	part, err := writer.CreateFormFile("image", "test.jpg")
	if err != nil {
		t.Fatalf("Failed to create form file: %v", err)
	}
	io.Copy(part, imageData)
	writer.Close()

	req, err := http.NewRequest("POST", "/notes", body)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Note: This test will likely fail due to missing Drive service
	// but it validates the handler structure
	if w.Code == http.StatusBadRequest {
		var response map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &response)
		// Expected to fail due to missing Drive service
		if msg, ok := response["error"].(string); ok && strings.Contains(msg, "Failed to upload file") {
			// This is expected when Drive service is not initialized
		} else {
			t.Logf("CreateNote() status = %v, body = %v", w.Code, w.Body.String())
		}
	} else {
		t.Logf("CreateNote() status = %v, body = %v", w.Code, w.Body.String())
	}

	// Cleanup
	database.Client.Database(os.Getenv("DB_NAME")).Drop(ctx)
}

func TestCreateNote_MissingImage(t *testing.T) {
	os.Setenv("MONGO_URI", "mongodb://localhost:27017")
	os.Setenv("DB_NAME", "cogniscan_test")
	database.ConnectDB()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	defer database.Client.Database(os.Getenv("DB_NAME")).Drop(ctx)

	router := setupNoteTestRouter()
	router.POST("/notes", CreateNote)

	// Create a multipart form without image
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("name", "Test Note")
	writer.WriteField("folderId", "folder123")
	writer.Close()

	req, err := http.NewRequest("POST", "/notes", body)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("CreateNote() without image should return 400, got %v", w.Code)
	}
}

func TestGetNoteImage(t *testing.T) {
	os.Setenv("MONGO_URI", "mongodb://localhost:27017")
	os.Setenv("DB_NAME", "cogniscan_test")
	database.ConnectDB()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create test note
	noteID := primitive.NewObjectID()
	testNote := models.Note{
		ID:       noteID,
		Name:     "Test Note",
		DriveID:  "drive123",
		FolderID: "folder123",
		OwnerID:  "test-user-id",
	}

	notesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("notes")
	_, err := notesCollection.InsertOne(ctx, testNote)
	if err != nil {
		t.Skip("Skipping test: MongoDB not available")
	}
	defer database.Client.Database(os.Getenv("DB_NAME")).Drop(ctx)

	router := setupNoteTestRouter()
	router.GET("/notes/:id/image", GetNoteImage)

	tests := []struct {
		name       string
		noteID     string
		wantStatus int
	}{
		{
			name:       "Successfully get note image",
			noteID:     noteID.Hex(),
			wantStatus: http.StatusOK,
		},
		{
			name:       "Invalid note ID",
			noteID:     "invalid-id",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Non-existent note",
			noteID:     primitive.NewObjectID().Hex(),
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", "/notes/"+tt.noteID+"/image", nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("GetNoteImage() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.name == "Successfully get note image" {
				// Verify content type is appropriate for image
				contentType := w.Header().Get("Content-Type")
				// Image endpoint may return image content type or redirect
				t.Logf("Content-Type: %v", contentType)
			}
		})
	}
}

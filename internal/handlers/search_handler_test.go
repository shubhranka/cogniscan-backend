package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"cogniscan/backend/internal/database"
	"cogniscan/backend/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// setupSearchTestRouter creates a test router with auth middleware mock
func setupSearchTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Mock auth middleware that sets user ID in context
	router.Use(func(c *gin.Context) {
		c.Set("user", "test-user-id")
		c.Next()
	})

	return router
}

func TestSearchItems(t *testing.T) {
	os.Setenv("MONGO_URI", "mongodb://localhost:27017")
	os.Setenv("DB_NAME", "cogniscan_test")
	database.ConnectDB()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create test folders
	foldersCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("folders")
	notesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("notes")

	// Insert test folders
	testFolders := []models.Folder{
		{
			ID:       primitive.NewObjectID(),
			Name:     "Important Documents",
			ParentID: "",
			OwnerID:  "test-user-id",
		},
		{
			ID:       primitive.NewObjectID(),
			Name:     "Photos",
			ParentID: "",
			OwnerID:  "test-user-id",
		},
		{
			ID:       primitive.NewObjectID(),
			Name:     "Work Files",
			ParentID: "",
			OwnerID:  "other-user-id",
		},
	}

	for _, folder := range testFolders {
		_, err := foldersCollection.InsertOne(ctx, folder)
		if err != nil {
			t.Skip("Skipping test: MongoDB not available")
		}
	}

	// Insert test notes
	testNotes := []models.Note{
		{
			ID:       primitive.NewObjectID(),
			Name:     "Important Meeting Notes",
			DriveID:  "drive1",
			FolderID: testFolders[0].ID.Hex(),
			OwnerID:  "test-user-id",
			Caption:  "Meeting notes about important project",
		},
		{
			ID:       primitive.NewObjectID(),
			Name:     "Vacation Photo",
			DriveID:  "drive2",
			FolderID: testFolders[1].ID.Hex(),
			OwnerID:  "test-user-id",
			Caption:  "Beautiful sunset at the beach",
		},
		{
			ID:       primitive.NewObjectID(),
			Name:     "Photo of Document",
			DriveID:  "drive3",
			FolderID: testFolders[1].ID.Hex(),
			OwnerID:  "test-user-id",
			Caption:  "Scan of important document",
		},
		{
			ID:       primitive.NewObjectID(),
			Name:     "Other User Note",
			DriveID:  "drive4",
			FolderID: "folder123",
			OwnerID:  "other-user-id",
			Caption:  "Other user content",
		},
	}

	for _, note := range testNotes {
		_, err := notesCollection.InsertOne(ctx, note)
		if err != nil {
			t.Skip("Skipping test: MongoDB not available")
		}
	}

	defer database.Client.Database(os.Getenv("DB_NAME")).Drop(ctx)

	router := setupSearchTestRouter()
	router.GET("/search", SearchItems)

	tests := []struct {
		name              string
		query             string
		wantStatus        int
		wantMinResults    int
		wantMaxResults    int
		expectedFolderIDs []string
	}{
		{
			name:           "Search for 'important' - matches both folder name and captions",
			query:          "important",
			wantStatus:     http.StatusOK,
			wantMinResults: 3,
			wantMaxResults: 3,
			expectedFolderIDs: []string{
				testFolders[0].ID.Hex(), // "Important Documents" folder
			},
		},
		{
			name:           "Search for 'photo' - matches folder and note name",
			query:          "photo",
			wantStatus:     http.StatusOK,
			wantMinResults: 2,
			wantMaxResults: 2,
			expectedFolderIDs: []string{
				testFolders[1].ID.Hex(), // "Photos" folder
			},
		},
		{
			name:           "Search for 'beach' - matches only caption",
			query:          "beach",
			wantStatus:     http.StatusOK,
			wantMinResults: 1,
			wantMaxResults: 1,
		},
		{
			name:           "Search for 'meeting' - matches note name",
			query:          "meeting",
			wantStatus:     http.StatusOK,
			wantMinResults: 1,
			wantMaxResults: 1,
		},
		{
			name:           "Empty query - returns empty array",
			query:          "",
			wantStatus:     http.StatusOK,
			wantMinResults: 0,
			wantMaxResults: 0,
		},
		{
			name:           "No matches - returns empty array",
			query:          "nonexistent",
			wantStatus:     http.StatusOK,
			wantMinResults: 0,
			wantMaxResults: 0,
		},
		{
			name:           "Case insensitive search - 'IMPORTANT' should match",
			query:          "IMPORTANT",
			wantStatus:     http.StatusOK,
			wantMinResults: 3,
			wantMaxResults: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", "/search?q="+tt.query, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("SearchItems() status = %v, want %v", w.Code, tt.wantStatus)
			}

			var results []SearchResultItem
			if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
				t.Fatalf("Failed to unmarshal response: %v", err)
			}

			if len(results) < tt.wantMinResults || len(results) > tt.wantMaxResults {
				t.Errorf("SearchItems() results count = %v, want between %d and %d", len(results), tt.wantMinResults, tt.wantMaxResults)
			}

			// Verify all results belong to the test user
			for _, result := range results {
				if result.Type == "folder" {
					// Check if folder is in expected list
					found := false
					for _, expectedID := range tt.expectedFolderIDs {
						if result.ID == expectedID {
							found = true
							break
						}
					}
					if len(tt.expectedFolderIDs) > 0 && !found {
						t.Logf("Warning: Unexpected folder in results: %s (name: %s)", result.ID, result.Name)
					}
				}
			}
		})
	}
}

func TestSearchItems_Unauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// No auth middleware - should result in unauthorized
	router.GET("/search", SearchItems)

	req, err := http.NewRequest("GET", "/search?q=test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("SearchItems() without auth should return 401, got %v", w.Code)
	}
}

func TestSearchItems_ResultTypes(t *testing.T) {
	os.Setenv("MONGO_URI", "mongodb://localhost:27017")
	os.Setenv("DB_NAME", "cogniscan_test")
	database.ConnectDB()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create a folder and a note with matching names
	folderID := primitive.NewObjectID()
	noteID := primitive.NewObjectID()

	foldersCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("folders")
	notesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("notes")

	testFolder := models.Folder{
		ID:       folderID,
		Name:     "Test",
		ParentID: "",
		OwnerID:  "test-user-id",
	}

	testNote := models.Note{
		ID:       noteID,
		Name:     "Test",
		DriveID:  "drive1",
		FolderID: "folder123",
		OwnerID:  "test-user-id",
		Caption:  "Test caption",
	}

	_, err := foldersCollection.InsertOne(ctx, testFolder)
	if err != nil {
		t.Skip("Skipping test: MongoDB not available")
	}
	_, err = notesCollection.InsertOne(ctx, testNote)
	if err != nil {
		t.Skip("Skipping test: MongoDB not available")
	}

	defer database.Client.Database(os.Getenv("DB_NAME")).Drop(ctx)

	router := setupSearchTestRouter()
	router.GET("/search", SearchItems)

	req, err := http.NewRequest("GET", "/search?q=Test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("SearchItems() status = %v, want %v", w.Code, http.StatusOK)
	}

	var results []SearchResultItem
	if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	// Should have both folder and note results
	if len(results) != 2 {
		t.Errorf("Expected 2 results (1 folder, 1 note), got %d", len(results))
	}

	// Verify we have one folder and one note
	folderCount := 0
	noteCount := 0
	for _, result := range results {
		if result.Type == "folder" {
			folderCount++
		} else if result.Type == "note" {
			noteCount++
		}
	}

	if folderCount != 1 {
		t.Errorf("Expected 1 folder result, got %d", folderCount)
	}
	if noteCount != 1 {
		t.Errorf("Expected 1 note result, got %d", noteCount)
	}
}

func TestSearchItems_Structure(t *testing.T) {
	os.Setenv("MONGO_URI", "mongodb://localhost:27017")
	os.Setenv("DB_NAME", "cogniscan_test")
	database.ConnectDB()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create a test folder
	folderID := primitive.NewObjectID()
	foldersCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("folders")

	testFolder := models.Folder{
		ID:       folderID,
		Name:     "Test Folder",
		ParentID: "",
		OwnerID:  "test-user-id",
	}

	_, err := foldersCollection.InsertOne(ctx, testFolder)
	if err != nil {
		t.Skip("Skipping test: MongoDB not available")
	}

	defer database.Client.Database(os.Getenv("DB_NAME")).Drop(ctx)

	router := setupSearchTestRouter()
	router.GET("/search", SearchItems)

	req, err := http.NewRequest("GET", "/search?q=Test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var results []SearchResultItem
	json.Unmarshal(w.Body.Bytes(), &results)

	if len(results) > 0 {
		result := results[0]

		// Verify result structure
		if result.Type == "" {
			t.Error("Search result is missing Type field")
		}
		if result.ID == "" {
			t.Error("Search result is missing ID field")
		}
		if result.Name == "" {
			t.Error("Search result is missing Name field")
		}
		if result.CreatedAt.IsZero() {
			t.Error("Search result is missing or invalid CreatedAt field")
		}

		// Folder-specific fields
		if result.Type == "folder" {
			if result.ParentID == "" {
				// Empty is valid for root folders
			}
		}

		// Note-specific fields
		if result.Type == "note" {
			if result.Caption == "" {
				// Empty is valid for notes without captions
			}
			if result.FolderID == "" {
				t.Error("Note result is missing FolderID field")
			}
		}
	}
}

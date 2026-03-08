package handlers

import (
	"bytes"
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
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// setupTestDB initializes a test database connection for folder tests
func setupTestDB(t *testing.T) {
	os.Setenv("MONGO_URI", "mongodb://localhost:27017")
	os.Setenv("DB_NAME", "cogniscan_test")
	database.ConnectDB()
}

func cleanupTestDB(t *testing.T) {
	if database.Client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		database.Client.Database(os.Getenv("DB_NAME")).Drop(ctx)
	}
}

// setupAuthContext creates a mock Firebase user in context
func setupAuthContext(c *gin.Context) {
	// Create a mock user ID that would come from Firebase
	c.Set("user", "test-user-id")
}

// setupTestRouter creates a test router with auth middleware mock
func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Mock auth middleware
	router.Use(func(c *gin.Context) {
		c.Set("user", "test-user-id")
		c.Next()
	})

	return router
}

func TestCreateFolder(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	router := setupTestRouter()
	router.POST("/folders", CreateFolder)

	tests := []struct {
		name       string
		payload    map[string]interface{}
		wantStatus int
		checkBody  bool
	}{
		{
			name: "Successfully create root folder",
			payload: map[string]interface{}{
				"name":     "Test Folder",
				"parentId": "",
			},
			wantStatus: http.StatusCreated,
			checkBody:  true,
		},
		{
			name: "Successfully create nested folder",
			payload: map[string]interface{}{
				"name":     "Nested Folder",
				"parentId": "parent123",
			},
			wantStatus: http.StatusCreated,
			checkBody:  true,
		},
		{
			name: "Missing required name field",
			payload: map[string]interface{}{
				"parentId": "parent123",
			},
			wantStatus: http.StatusBadRequest,
			checkBody:  false,
		},
		{
			name: "Empty payload",
			payload: map[string]interface{}{},
			wantStatus: http.StatusBadRequest,
			checkBody:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payloadBytes, err := json.Marshal(tt.payload)
			if err != nil {
				t.Fatalf("Failed to marshal payload: %v", err)
			}

			req, err := http.NewRequest("POST", "/folders", bytes.NewBuffer(payloadBytes))
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("CreateFolder() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.checkBody && w.Code == http.StatusCreated {
				var response models.Folder
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Fatalf("Failed to unmarshal response: %v", err)
				}

				if response.Name != tt.payload["name"] {
					t.Errorf("Expected name %v, got %v", tt.payload["name"], response.Name)
				}
				if response.OwnerID != "test-user-id" {
					t.Errorf("Expected ownerId 'test-user-id', got %v", response.OwnerID)
				}
			}
		})
	}
}

func TestGetFolders(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	router := setupTestRouter()

	// Create test folders
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	foldersCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("folders")

	testFolders := []models.Folder{
		{
			ID:       primitive.NewObjectID(),
			Name:     "Folder 1",
			ParentID: "",
			OwnerID:  "test-user-id",
		},
		{
			ID:       primitive.NewObjectID(),
			Name:     "Folder 2",
			ParentID: "",
			OwnerID:  "test-user-id",
		},
		{
			ID:       primitive.NewObjectID(),
			Name:     "Nested Folder",
			ParentID: "parent123",
			OwnerID:  "test-user-id",
		},
		{
			ID:       primitive.NewObjectID(),
			Name:     "Other User Folder",
			ParentID: "",
			OwnerID:  "other-user-id",
		},
	}

	for _, folder := range testFolders {
		_, err := foldersCollection.InsertOne(ctx, folder)
		if err != nil {
			t.Fatalf("Failed to insert test folder: %v", err)
		}
	}

	router.GET("/folders/:folderId", GetFolders)

	tests := []struct {
		name       string
		folderId   string
		wantStatus int
		minCount   int
		maxCount   int
	}{
		{
			name:       "Get root folders",
			folderId:   "root",
			wantStatus: http.StatusOK,
			minCount:   2,
			maxCount:   2,
		},
		{
			name:       "Get nested folders",
			folderId:   "parent123",
			wantStatus: http.StatusOK,
			minCount:   1,
			maxCount:   1,
		},
		{
			name:       "Get folders with empty parent ID",
			folderId:   "",
			wantStatus: http.StatusOK,
			minCount:   2,
			maxCount:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", "/folders/"+tt.folderId, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("GetFolders() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.minCount >= 0 {
				var response []models.Folder
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Fatalf("Failed to unmarshal response: %v", err)
				}

				if len(response) < tt.minCount || len(response) > tt.maxCount {
					t.Errorf("Expected between %d and %d folders, got %d", tt.minCount, tt.maxCount, len(response))
				}

				// Verify all folders belong to the test user
				for _, folder := range response {
					if folder.OwnerID != "test-user-id" {
						t.Errorf("Folder %s belongs to wrong user: %s", folder.Name, folder.OwnerID)
					}
				}
			}
		})
	}
}

func TestUpdateFolder(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	router := setupTestRouter()

	// Create a test folder
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	folderID := primitive.NewObjectID()
	testFolder := models.Folder{
		ID:       folderID,
		Name:     "Original Name",
		ParentID: "",
		OwnerID:  "test-user-id",
	}

	foldersCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("folders")
	_, err := foldersCollection.InsertOne(ctx, testFolder)
	if err != nil {
		t.Fatalf("Failed to insert test folder: %v", err)
	}

	router.PUT("/folders/:id", UpdateFolder)

	tests := []struct {
		name       string
		folderID   string
		payload    map[string]interface{}
		wantStatus int
	}{
		{
			name:     "Successfully update folder",
			folderID: folderID.Hex(),
			payload: map[string]interface{}{
				"name": "Updated Name",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:     "Invalid folder ID",
			folderID: "invalid-id",
			payload: map[string]interface{}{
				"name": "Updated Name",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:     "Non-existent folder",
			folderID: primitive.NewObjectID().Hex(),
			payload: map[string]interface{}{
				"name": "Updated Name",
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:     "Missing required name field",
			folderID: folderID.Hex(),
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

			req, err := http.NewRequest("PUT", "/folders/"+tt.folderID, bytes.NewBuffer(payloadBytes))
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("UpdateFolder() status = %v, want %v", w.Code, tt.wantStatus)
			}

			// Verify update in database
			if tt.name == "Successfully update folder" {
				var updatedFolder models.Folder
				err = foldersCollection.FindOne(ctx, bson.M{"_id": folderID}).Decode(&updatedFolder)
				if err != nil {
					t.Fatalf("Failed to find updated folder: %v", err)
				}

				if updatedFolder.Name != "Updated Name" {
					t.Errorf("Expected name 'Updated Name', got %s", updatedFolder.Name)
				}
			}
		})
	}
}

func TestDeleteFolder(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	router := setupTestRouter()

	// Create test folders and notes
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	foldersCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("folders")
	notesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("notes")

	// Create parent folder with nested folder and notes
	parentID := primitive.NewObjectID()
	subFolderID := primitive.NewObjectID()

	parentFolder := models.Folder{
		ID:       parentID,
		Name:     "Parent Folder",
		ParentID: "",
		OwnerID:  "test-user-id",
	}

	subFolder := models.Folder{
		ID:       subFolderID,
		Name:     "Sub Folder",
		ParentID: parentID.Hex(),
		OwnerID:  "test-user-id",
	}

	note1 := models.Note{
		ID:       primitive.NewObjectID(),
		Name:     "Note 1",
		DriveID:  "drive1",
		FolderID: parentID.Hex(),
		OwnerID:  "test-user-id",
	}

	note2 := models.Note{
		ID:       primitive.NewObjectID(),
		Name:     "Note 2",
		DriveID:  "drive2",
		FolderID: subFolderID.Hex(),
		OwnerID:  "test-user-id",
	}

	_, err := foldersCollection.InsertOne(ctx, parentFolder)
	if err != nil {
		t.Fatalf("Failed to insert parent folder: %v", err)
	}
	_, err = foldersCollection.InsertOne(ctx, subFolder)
	if err != nil {
		t.Fatalf("Failed to insert sub folder: %v", err)
	}
	_, err = notesCollection.InsertOne(ctx, note1)
	if err != nil {
		t.Fatalf("Failed to insert note 1: %v", err)
	}
	_, err = notesCollection.InsertOne(ctx, note2)
	if err != nil {
		t.Fatalf("Failed to insert note 2: %v", err)
	}

	router.DELETE("/folders/:id", DeleteFolder)

	tests := []struct {
		name       string
		folderID   string
		wantStatus int
	}{
		{
			name:       "Successfully delete folder with contents",
			folderID:   parentID.Hex(),
			wantStatus: http.StatusOK,
		},
		{
			name:       "Invalid folder ID",
			folderID:   "invalid-id",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Non-existent folder",
			folderID:   primitive.NewObjectID().Hex(),
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("DELETE", "/folders/"+tt.folderID, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("DeleteFolder() status = %v, want %v", w.Code, tt.wantStatus)
			}

			// Verify deletion in database for successful case
			if tt.name == "Successfully delete folder with contents" {
				count, _ := foldersCollection.CountDocuments(ctx, bson.M{"_id": parentID})
				if count > 0 {
					t.Error("Parent folder should be deleted")
				}
				count, _ = foldersCollection.CountDocuments(ctx, bson.M{"_id": subFolderID})
				if count > 0 {
					t.Error("Sub folder should be deleted")
				}
				count, _ = notesCollection.CountDocuments(ctx, bson.M{"folderId": parentID.Hex()})
				if count > 0 {
					t.Error("Notes in parent folder should be deleted")
				}
				count, _ = notesCollection.CountDocuments(ctx, bson.M{"folderId": subFolderID.Hex()})
				if count > 0 {
					t.Error("Notes in sub folder should be deleted")
				}
			}
		})
	}
}

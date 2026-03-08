package services

import (
	"context"
	"os"
	"testing"
	"time"

	"cogniscan/backend/internal/database"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// setupTestDB initializes a test database connection
func setupTestDB(t *testing.T) {
	os.Setenv("MONGO_URI", "mongodb://localhost:27017")
	os.Setenv("DB_NAME", "cogniscan_test")

	database.ConnectDB()
	if database.Client == nil {
		t.Skip("Skipping test: MongoDB not available")
	}
}

func cleanupTestDB() {
	if database.Client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		database.Client.Database(os.Getenv("DB_NAME")).Drop(ctx)
	}
}

func TestUpdateNoteCaption(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB()

	// Create a test note
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	noteID := primitive.NewObjectID()
	testNote := bson.M{
		"_id":      noteID,
		"name":     "Test Note",
		"driveId":  "drive123",
		"caption":  "Original caption",
		"ownerId":  "user123",
		"folderId": "folder123",
	}

	notesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("notes")
	_, err := notesCollection.InsertOne(ctx, testNote)
	if err != nil {
		t.Fatalf("Failed to insert test note: %v", err)
	}

	tests := []struct {
		name      string
		noteID    string
		caption   string
		wantErr   bool
		setup     func()
	}{
		{
			name:    "Successfully update caption",
			noteID:  noteID.Hex(),
			caption: "New updated caption",
			wantErr: false,
		},
		{
			name:    "Update with empty caption",
			noteID:  noteID.Hex(),
			caption: "",
			wantErr: false,
		},
		{
			name:    "Invalid note ID",
			noteID:  "invalid-id",
			caption: "Test caption",
			wantErr: true,
		},
		{
			name:    "Non-existent note",
			noteID:  primitive.NewObjectID().Hex(),
			caption: "Test caption",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup()
			}

			err := UpdateNoteCaption(tt.noteID, tt.caption)

			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateNoteCaption() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.noteID == noteID.Hex() {
				// Verify the update
				var result bson.M
				err = notesCollection.FindOne(ctx, bson.M{"_id": noteID}).Decode(&result)
				if err != nil {
					t.Errorf("Failed to retrieve updated note: %v", err)
					return
				}

				if result["caption"] != tt.caption {
					t.Errorf("Expected caption '%s', got '%s'", tt.caption, result["caption"])
				}
			}
		})
	}
}

func TestUpdateNoteCaption_Concurrent(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	noteID := primitive.NewObjectID()
	testNote := bson.M{
		"_id":      noteID,
		"name":     "Test Note",
		"driveId":  "drive123",
		"caption":  "Original caption",
		"ownerId":  "user123",
		"folderId": "folder123",
	}

	notesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("notes")
	_, err := notesCollection.InsertOne(ctx, testNote)
	if err != nil {
		t.Fatalf("Failed to insert test note: %v", err)
	}

	// Test concurrent updates
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			UpdateNoteCaption(noteID.Hex(), "Caption "+string(rune('0'+idx)))
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify final state
	var result bson.M
	err = notesCollection.FindOne(ctx, bson.M{"_id": noteID}).Decode(&result)
	if err != nil {
		t.Fatalf("Failed to retrieve updated note: %v", err)
	}

	if result["caption"] == "" {
		t.Error("Caption should have been updated")
	}
}

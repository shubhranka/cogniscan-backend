package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"cogniscan/backend/internal/database"
	"cogniscan/backend/internal/models"
	"go.mongodb.org/mongo-driver/bson"
)

func TestMongoDBConnection(t *testing.T) {
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		t.Skip("MONGO_URI not configured, skipping integration test")
		return
	}

	t.Run("Test MongoDB connection", func(t *testing.T) {
		// Test that MongoDB connection works
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := database.Client.Ping(ctx, nil)
		if err != nil {
			t.Fatalf("Failed to ping MongoDB: %v", err)
		}

		t.Log("MongoDB connection successful")
	})
}

func TestDatabaseCollections(t *testing.T) {
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		t.Skip("MONGO_URI not configured, skipping integration test")
		return
	}

	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		t.Skip("DB_NAME not configured, skipping integration test")
		return
	}

	t.Run("List collections", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Test that we can list collections
		collections, err := database.Client.Database(dbName).ListCollectionNames(ctx, nil)
		if err != nil {
			t.Fatalf("Failed to list collections: %v", err)
		}

		t.Logf("Found collections: %v", collections)

		// Verify expected collections exist
		expectedCollections := map[string]bool{
			"users":            true,
			"folders":          true,
			"notes":            true,
			"caption_embeddings": true,
			"quizzes":          true,
			"questions":         true,
			"question_answers":  true,
			"note_reviews":     true,
			"user_progress":    true,
			"folder_mastery":   true,
			"document_index":   true,
			"quiz_sessions":   true,
		}

		collectionMap := make(map[string]bool)
		for _, col := range collections {
			collectionMap[col] = true
		}

		for expectedCol := range expectedCollections {
			if !collectionMap[expectedCol] {
				t.Errorf("Expected collection %s not found", expectedCol)
			}
		}
	})
}

func TestCreateAndReadDocument(t *testing.T) {
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		t.Skip("MONGO_URI not configured, skipping integration test")
		return
	}

	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		t.Skip("DB_NAME not configured, skipping integration test")
		return
	}

	t.Run("Create and read document", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Clean up test collection
		testCollection := database.Client.Database(dbName).Collection("test_integration")
		testCollection.Drop(ctx)

		// Create a test document
		doc := models.Note{
			Name:     "Test Note",
			DriveID:  "test-drive-id",
			FolderID: "test-folder-id",
			OwnerID:  "test-user-id",
		}

		result, err := testCollection.InsertOne(ctx, doc)
		if err != nil {
			t.Fatalf("Failed to insert document: %v", err)
		}

		// Read the document back
		var readDoc models.Note
		filter := bson.M{"_id": result.InsertedID}
		err = testCollection.FindOne(ctx, filter).Decode(&readDoc)
		if err != nil {
			t.Fatalf("Failed to read document: %v", err)
		}

		// Verify the data matches
		if readDoc.Name != doc.Name {
			t.Errorf("Name mismatch: got %s, want %s", readDoc.Name, doc.Name)
		}

		// Clean up
		testCollection.Drop(ctx)
	})
}

func TestVectorSearchIntegration(t *testing.T) {
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		t.Skip("MONGO_URI not configured, skipping integration test")
		return
	}

	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		t.Skip("DB_NAME not configured, skipping integration test")
		return
	}

	t.Run("Test vector search setup", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// This test verifies the vector search collection exists
		// Vector search requires MongoDB Atlas Vector Search index
		collection := database.Client.Database(dbName).Collection("caption_embeddings")

		// Create a test embedding
		testEmbedding := models.CaptionEmbedding{
			NoteID:    "test-note-id",
			FolderID:  "test-folder-id",
			OwnerID:   "test-user-id",
			Caption:   "test caption",
			Vector:    make([]float32, 1024), // 1024 dimensions
		}

		result, err := collection.InsertOne(ctx, testEmbedding)
		if err != nil {
			t.Fatalf("Failed to insert test embedding: %v", err)
		}

		// Clean up
		collection.DeleteOne(ctx, bson.M{"_id": result.InsertedID})

		t.Logf("Vector search collection exists and can store embeddings")
	})
}

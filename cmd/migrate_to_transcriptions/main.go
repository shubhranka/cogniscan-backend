// main package for migrate to transcriptions script
// This script resets existing captioned notes to "pending" status so they get regenerated with the new transcription prompt
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Note struct {
	ID            string `bson:"_id"`
	Name          string `bson:"name"`
	DriveID       string `bson:"driveId"`
	Caption       string `bson:"caption"`
	CaptionStatus string `bson:"captionStatus"`
	CaptionError  string `bson:"captionError"`
	FolderID      string `bson:"folderId"`
	OwnerID       string `bson:"ownerId"`
}

// Use dotenv to load environment variables
func init() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: Error loading .env file")
	}
}

var mongoClient *mongo.Client

func main() {
	log.Println("=== Migrate to Full-Text Transcriptions ===")
	log.Println("This script will reset existing captioned notes to 'pending' status.")
	log.Println("The caption worker will regenerate them with the new OCR-style transcription prompt.")
	log.Println("")

	// Initialize MongoDB
	if err := initMongoDB(); err != nil {
		log.Fatalf("Failed to initialize MongoDB: %v", err)
	}

	// Find notes with existing captions
	notes, err := findNotesWithCaptions()
	if err != nil {
		log.Fatalf("Failed to find notes: %v", err)
	}

	if len(notes) == 0 {
		log.Println("No notes found with existing captions. Nothing to migrate.")
		return
	}

	log.Printf("Found %d notes with existing captions\n", len(notes))

	// Show preview of first few notes
	previewCount := len(notes)
	if previewCount > 5 {
		previewCount = 5
	}
	log.Println("Preview of notes to be regenerated:")
	for i := 0; i < previewCount; i++ {
		log.Printf("  - %s (ID: %s)", notes[i].Name, notes[i].ID)
	}
	if len(notes) > 5 {
		log.Printf("  ... and %d more", len(notes)-5)
	}

	// Ask for confirmation
	fmt.Printf("\nRegenerate transcriptions for %d notes? (y/n): ", len(notes))
	var confirmation string
	fmt.Scanln(&confirmation)
	if confirmation != "y" && confirmation != "Y" {
		log.Println("Aborted by user")
		return
	}

	// Process each note - reset to pending status
	successCount := 0
	failureCount := 0

	for i, note := range notes {
		log.Printf("\n[%d/%d] Resetting note: %s (ID: %s)", i+1, len(notes), note.Name, note.ID)

		if err := resetNoteStatus(note); err != nil {
			log.Printf("  ERROR: Failed to reset status: %v", err)
			failureCount++
		} else {
			log.Printf("  SUCCESS: Reset to pending status")
			successCount++
		}

		// Small delay to avoid overwhelming database
		if i < len(notes)-1 {
			time.Sleep(50 * time.Millisecond)
		}
	}

	log.Printf("\n=== Migration Summary ===")
	log.Printf("Total notes processed: %d", len(notes))
	log.Printf("Successfully reset: %d", successCount)
	log.Printf("Failed: %d", failureCount)
	log.Println("\nNotes will be processed by the caption worker.")
	log.Println("Check the worker logs for regeneration progress.")
}

func initMongoDB() error {
	log.Println("Initializing MongoDB...")
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		return fmt.Errorf("MONGO_URI environment variable not set")
	}

	client, err := mongo.NewClient(options.Client().ApplyURI(mongoURI))
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		return err
	}

	if err := client.Ping(ctx, nil); err != nil {
		return err
	}

	mongoClient = client
	log.Println("  MongoDB connected")
	return nil
}

func findNotesWithCaptions() ([]Note, error) {
	log.Println("Finding notes with existing captions...")

	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "cogniscan"
	}

	notesCollection := mongoClient.Database(dbName).Collection("notes")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Find notes where caption exists and is not empty
	filter := bson.M{
		"caption": bson.M{"$ne": ""},
	}

	cursor, err := notesCollection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var notes []Note
	if err := cursor.All(ctx, &notes); err != nil {
		return nil, err
	}

	log.Printf("  Found %d notes with captions", len(notes))
	return notes, nil
}

func resetNoteStatus(note Note) error {
	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "cogniscan"
	}

	notesCollection := mongoClient.Database(dbName).Collection("notes")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	objID, err := primitive.ObjectIDFromHex(note.ID)
	if err != nil {
		return err
	}

	// Reset status to pending and clear error
	filter := bson.M{"_id": objID}
	update := bson.M{
		"$set": bson.M{
			"captionStatus": "pending",
			"captionError":  "",
			"updatedAt":     time.Now(),
		},
	}

	result, err := notesCollection.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("note not found")
	}

	return nil
}

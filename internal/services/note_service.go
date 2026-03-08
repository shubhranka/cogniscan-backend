package services

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"cogniscan/backend/internal/database"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// UpdateNoteCaption updates the caption field of a note
func UpdateNoteCaption(noteID string, caption string) error {
	notesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("notes")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	objID, err := primitive.ObjectIDFromHex(noteID)
	if err != nil {
		return err
	}

	filter := bson.M{"_id": objID}
	update := bson.M{"$set": bson.M{"caption": caption, "updatedAt": time.Now()}}

	result, err := notesCollection.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("note not found")
	}

	log.Printf("[NoteService] Updated caption for note %s", noteID)
	return nil
}

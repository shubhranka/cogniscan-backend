package migrations

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"cogniscan/backend/internal/database"
	"cogniscan/backend/internal/models"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// MigrateToNodes migrates data from folders and notes collections to nodes collection
func MigrateToNodes(ctx context.Context) error {
	godotenv.Load()
	dbName := os.Getenv("DB_NAME")
	db := database.Client.Database(dbName)

	log.Println("Starting migration to nodes collection...")

	// Step 1: Create nodes collection with indexes
	nodesCollection := db.Collection("nodes")

	// Create indexes
	indexes := []mongo.IndexModel{
		{Keys: bson.D{{Key: "parentId", Value: 1}}},
		{Keys: bson.D{{Key: "ownerId", Value: 1}}},
		{Keys: bson.D{{Key: "metadata.type", Value: 1}}},
		{Keys: bson.D{{Key: "children", Value: 1}}},
	}

	_, err := nodesCollection.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}
	log.Println("Created indexes on nodes collection")

	// Step 2: Migrate folders as folder-type nodes
	foldersCollection := db.Collection("folders")
	folderCursor, err := foldersCollection.Find(ctx, bson.M{})
	if err != nil {
		return fmt.Errorf("failed to fetch folders: %w", err)
	}
	defer folderCursor.Close(ctx)

	var folders []models.Folder
	if err := folderCursor.All(ctx, &folders); err != nil {
		return fmt.Errorf("failed to decode folders: %w", err)
	}

	log.Printf("Migrating %d folders to nodes...", len(folders))

	var folderIDMap = make(map[string]string) // old folder ID -> new node ID
	var nodeCount int

	for _, folder := range folders {
		now := time.Now()

		// Create folder node with initial mastery
		node := models.Node{
			ID:       folder.ID,
			Name:     folder.Name,
			ParentID: folder.ParentID,
			Children: []string{},
			Metadata: models.NodeMetadata{
				Type: models.NodeTypeFolder,
			},
			OwnerID:              folder.OwnerID,
			CreatedAt:            folder.CreatedAt,
			UpdatedAt:            folder.UpdatedAt,
			QuizGenerationStatus: folder.QuizGenerationStatus,
			QuizID:               folder.QuizID,
			QuizError:            folder.QuizError,
			QuizUpdatedAt:        folder.QuizUpdatedAt,
			TotalNoteCount:       0, // Will be calculated later
			Mastery: models.NodeMastery{
				TotalNotes:     0,
				MasteredNotes:  0,
				LearntNotes:    0,
				MasteryLevel:   "Review Soon",
				MasteryPercent: 0,
				LastStudyDate:  now,
				LastUpdated:    now,
			},
		}

		_, err := nodesCollection.InsertOne(ctx, node)
		if err != nil {
			log.Printf("Failed to insert folder node %s: %v", folder.ID.Hex(), err)
			continue
		}

		folderIDMap[folder.ID.Hex()] = folder.ID.Hex()
		nodeCount++
	}

	log.Printf("Migrated %d folders to nodes", nodeCount)

	// Step 3: Migrate notes as note-type nodes
	notesCollection := db.Collection("notes")
	noteCursor, err := notesCollection.Find(ctx, bson.M{})
	if err != nil {
		return fmt.Errorf("failed to fetch notes: %w", err)
	}
	defer noteCursor.Close(ctx)

	var notes []models.Note
	if err := noteCursor.All(ctx, &notes); err != nil {
		return fmt.Errorf("failed to decode notes: %w", err)
	}

	log.Printf("Migrating %d notes to nodes...", len(notes))

	var noteCount int
	var noteIDMap = make(map[string]string) // old note ID -> new node ID

	for _, note := range notes {
		now := time.Now()

		// Get existing mastery from folder_mastery if available
		// For now, initialize with default mastery
		node := models.Node{
			ID:   note.ID,
			Name: note.Name,
			// Map folderId to parentId for notes
			ParentID: note.FolderID,
			Children: []string{},
			Metadata: models.NodeMetadata{
				Type:    models.NodeTypeNote,
				DriveID: note.DriveID,
			},
			OwnerID:        note.OwnerID,
			CreatedAt:      note.CreatedAt,
			UpdatedAt:      note.UpdatedAt,
			PublicURL:      note.PublicURL,
			// Caption data is kept in caption_embeddings collection, not in nodes
			TotalNoteCount: 1,
			Mastery: models.NodeMastery{
				TotalNotes:     1,
				MasteredNotes:  0,
				LearntNotes:    0,
				MasteryLevel:   "Review Soon",
				MasteryPercent: 0,
				LastStudyDate:  now,
				LastUpdated:    now,
			},
		}

		_, err := nodesCollection.InsertOne(ctx, node)
		if err != nil {
			log.Printf("Failed to insert note node %s: %v", note.ID.Hex(), err)
			continue
		}

		noteIDMap[note.ID.Hex()] = note.ID.Hex()
		noteCount++
	}

	log.Printf("Migrated %d notes to nodes", noteCount)

	// Step 4: Update children arrays for folders
	log.Println("Updating children arrays for folders...")
	for _, folder := range folders {
		// Find all notes that belonged to this folder
		filter := bson.M{
			"parentId":      folder.ID.Hex(),
			"metadata.type": models.NodeTypeNote,
		}
		cursor, err := nodesCollection.Find(ctx, filter)
		if err != nil {
			log.Printf("Failed to find children for folder %s: %v", folder.ID.Hex(), err)
			continue
		}
		defer cursor.Close(ctx)

		var children []models.Node
		if err := cursor.All(ctx, &children); err != nil {
			log.Printf("Failed to decode children for folder %s: %v", folder.ID.Hex(), err)
			continue
		}

		// Also find subfolder children
		subfolderFilter := bson.M{
			"parentId":      folder.ID.Hex(),
			"metadata.type": models.NodeTypeFolder,
		}
		subfolderCursor, err := nodesCollection.Find(ctx, subfolderFilter)
		if err != nil {
			log.Printf("Failed to find subfolder children for folder %s: %v", folder.ID.Hex(), err)
		} else {
			defer subfolderCursor.Close(ctx)

			var subfolderChildren []models.Node
			if err := subfolderCursor.All(ctx, &subfolderChildren); err != nil {
				log.Printf("Failed to decode subfolder children for folder %s: %v", folder.ID.Hex(), err)
			} else {
				children = append(children, subfolderChildren...)
			}
		}

		// Build children array
		childrenIDs := make([]string, len(children))
		for i, child := range children {
			childrenIDs[i] = child.ID.Hex()
		}

		// Update folder with children array
		_, err = nodesCollection.UpdateOne(
			ctx,
			bson.M{"_id": folder.ID},
			bson.M{"$set": bson.M{"children": childrenIDs}},
		)
		if err != nil {
			log.Printf("Failed to update children for folder %s: %v", folder.ID.Hex(), err)
		}
	}

	// Step 5: Calculate totalNoteCount for all nodes
	log.Println("Calculating totalNoteCount for all nodes...")
	err = calculateTotalNoteCounts(ctx, db)
	if err != nil {
		return fmt.Errorf("failed to calculate totalNoteCounts: %w", err)
	}

	// Step 6: Migrate mastery data from folder_mastery collection
	log.Println("Migrating mastery data...")
	err = migrateMasteryData(ctx, db)
	if err != nil {
		return fmt.Errorf("failed to migrate mastery data: %w", err)
	}

	// Step 7: Drop old collections after successful migration
	log.Println("Dropping old collections...")

	oldCollections := []string{"folders", "notes", "folder_mastery"}
	for _, collectionName := range oldCollections {
		err := db.Collection(collectionName).Drop(ctx)
		if err != nil {
			log.Printf("Warning: Failed to drop collection %s: %v", collectionName, err)
		} else {
			log.Printf("Dropped collection: %s", collectionName)
		}
	}

	log.Println("Migration completed successfully!")
	log.Printf("Total nodes created: %d (%d folders + %d notes)", nodeCount+noteCount, nodeCount, noteCount)
	log.Println("Old collections (folders, notes, folder_mastery) have been dropped")

	return nil
}

// calculateTotalNoteCounts calculates totalNoteCount for all nodes recursively
func calculateTotalNoteCounts(ctx context.Context, db *mongo.Database) error {
	nodesCollection := db.Collection("nodes")

	// Get all folders
	cursor, err := nodesCollection.Find(ctx, bson.M{"metadata.type": models.NodeTypeFolder})
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)

	var folders []models.Node
	if err := cursor.All(ctx, &folders); err != nil {
		return err
	}

	// Calculate totalNoteCount for each folder
	for _, folder := range folders {
		count := countDescendantNotes(ctx, nodesCollection, folder.ID.Hex(), 0)

		_, err := nodesCollection.UpdateOne(
			ctx,
			bson.M{"_id": folder.ID},
			bson.M{"$set": bson.M{"totalNoteCount": count}},
		)
		if err != nil {
			log.Printf("Failed to update totalNoteCount for folder %s: %v", folder.ID.Hex(), err)
		}
	}

	return nil
}

// countDescendantNotes counts note nodes under a given node
func countDescendantNotes(ctx context.Context, collection *mongo.Collection, nodeID string, depth int) int {
	if depth > 20 { // Prevent infinite loops
		return 0
	}

	var node models.Node
	err := collection.FindOne(ctx, bson.M{"_id": nodeID}).Decode(&node)
	if err != nil {
		return 0
	}

	count := 0

	// Count notes in this folder
	if node.Metadata.Type == models.NodeTypeFolder {
		for _, childID := range node.Children {
			var child models.Node
			err := collection.FindOne(ctx, bson.M{"_id": childID}).Decode(&child)
			if err != nil {
				continue
			}

			if child.Metadata.Type == models.NodeTypeNote {
				count++
			} else {
				// Recursively count notes in subfolder
				count += countDescendantNotes(ctx, collection, childID, depth+1)
			}
		}
	}

	return count
}

// migrateMasteryData migrates mastery data from folder_mastery collection to nodes
func migrateMasteryData(ctx context.Context, db *mongo.Database) error {
	folderMasteryCollection := db.Collection("folder_mastery")
	nodesCollection := db.Collection("nodes")

	cursor, err := folderMasteryCollection.Find(ctx, bson.M{})
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)

	var masteryRecords []models.FolderMastery
	if err := cursor.All(ctx, &masteryRecords); err != nil {
		return err
	}

	log.Printf("Migrating %d mastery records...", len(masteryRecords))

	for _, record := range masteryRecords {
		// Find the corresponding folder node
		var node models.Node
		err := nodesCollection.FindOne(ctx, bson.M{"_id": record.FolderID}).Decode(&node)
		if err != nil {
			log.Printf("Folder not found for mastery record %s: %v", record.FolderID, err)
			continue
		}

		// Update node with mastery data
		mastery := models.NodeMastery{
			TotalNotes:     record.TotalNotes,
			MasteredNotes:  record.MasteredNotes,
			LearntNotes:    record.LearntNotes,
			MasteryLevel:   record.MasteryLevel,
			MasteryPercent: record.MasteryPercent,
			LastStudyDate:  record.LastStudyDate,
			LastUpdated:    record.UpdatedAt,
		}

		_, err = nodesCollection.UpdateOne(
			ctx,
			bson.M{"_id": record.FolderID},
			bson.M{"$set": bson.M{"mastery": mastery}},
		)
		if err != nil {
			log.Printf("Failed to update mastery for folder %s: %v", record.FolderID, err)
		}
	}

	// Migrate note mastery from note_reviews
	noteReviewsCollection := db.Collection("note_reviews")
	reviewCursor, err := noteReviewsCollection.Find(ctx, bson.M{})
	if err != nil {
		return err
	}
	defer reviewCursor.Close(ctx)

	var reviews []models.NoteReview
	if err := reviewCursor.All(ctx, &reviews); err != nil {
		return err
	}

	log.Printf("Migrating %d note review records...", len(reviews))

	for _, review := range reviews {
		// Find the corresponding note node
		var node models.Node
		err := nodesCollection.FindOne(ctx, bson.M{"_id": review.NoteID}).Decode(&node)
		if err != nil {
			continue
		}

		// Calculate mastery for this note
		now := time.Now()
		mastery := models.NodeMastery{
			TotalNotes:     1,
			MasteredNotes:  0,
			LearntNotes:    0,
			MasteryLevel:   "Review Soon",
			MasteryPercent: 0,
			LastStudyDate:  now,
			LastUpdated:    now,
		}

		if review.Repetitions >= 3 {
			mastery.MasteredNotes = 1
			mastery.MasteryPercent = 1.0
			mastery.MasteryLevel = "Mastered"
		} else if review.Repetitions >= 1 {
			mastery.LearntNotes = 1
			mastery.MasteryPercent = 0.5
			mastery.MasteryLevel = "Learnt"
		}

		if !review.UpdatedAt.IsZero() {
			mastery.LastStudyDate = review.UpdatedAt
		}

		_, err = nodesCollection.UpdateOne(
			ctx,
			bson.M{"_id": review.NoteID},
			bson.M{"$set": bson.M{"mastery": mastery}},
		)
		if err != nil {
			log.Printf("Failed to update mastery for note %s: %v", review.NoteID, err)
		}
	}

	return nil
}

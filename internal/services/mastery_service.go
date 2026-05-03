package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"cogniscan/backend/internal/database"
	"cogniscan/backend/internal/models"
	"cogniscan/backend/internal/queue"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// Global mastery queue instance
var masteryQueue *queue.MasteryQueue

// SetMasteryQueue sets the global mastery queue instance
func SetMasteryQueue(q *queue.MasteryQueue) {
	masteryQueue = q
}

// EnqueueAncestorMasteryUpdate enqueues a background job to update ancestor mastery
func EnqueueAncestorMasteryUpdate(nodeID string) {
	if masteryQueue != nil {
		job := &queue.MasteryUpdateJob{
			ID:     queue.JobID(),
			NodeID: nodeID,
		}
		if err := masteryQueue.Enqueue(job); err != nil {
			log.Printf("Failed to enqueue ancestor mastery update: %v", err)
		} else {
			log.Printf("Enqueued ancestor mastery update for node: %s", nodeID)
		}
	}
}

var (
	ErrNodeNotFound    = errors.New("node not found")
	ErrInvalidNodeType = errors.New("invalid node type")
	ErrParentNotFound  = errors.New("parent node not found")
)

// GetNodesCollection returns the nodes collection
func GetNodesCollection() *mongo.Collection {
	return database.Client.Database(os.Getenv("DB_NAME")).Collection("nodes")
}

// GetNoteReviewsCollection returns the note_reviews collection
func GetNoteReviewsCollection() *mongo.Collection {
	return database.Client.Database(os.Getenv("DB_NAME")).Collection("note_reviews")
}

// DetermineMasteryLevel returns the mastery level based on percentage
func DetermineMasteryLevel(percent float64) string {
	if percent > 0.8 {
		return "Mastered"
	} else if percent >= 0.5 {
		return "Learnt"
	}
	return "Review Soon"
}

// CalculateNoteMastery computes mastery for a single note node based on its reviews
func CalculateNoteMastery(ctx context.Context, nodeID string) (*models.NodeMastery, error) {
	nodesCollection := GetNodesCollection()

	// Get the note node
	var node models.Node
	err := nodesCollection.FindOne(ctx, bson.M{"_id": nodeID}).Decode(&node)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrNodeNotFound
		}
		return nil, fmt.Errorf("failed to fetch node: %w", err)
	}

	if node.Metadata.Type != models.NodeTypeNote {
		return nil, ErrInvalidNodeType
	}

	// Get review for this note
	reviewsCollection := GetNoteReviewsCollection()
	var review models.NoteReview
	err = reviewsCollection.FindOne(ctx, bson.M{"noteId": nodeID, "userId": node.OwnerID}).Decode(&review)

	now := time.Now()
	mastery := &models.NodeMastery{
		TotalNotes:     1,
		MasteredNotes:  0,
		LearntNotes:    0,
		MasteryLevel:   "Review Soon",
		MasteryPercent: 0,
		LastStudyDate:  now,
		LastUpdated:    now,
	}

	if err == nil {
		// Review exists
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
	}

	return mastery, nil
}

// CalculateFolderMastery computes mastery for a folder node based on its children
func CalculateFolderMastery(ctx context.Context, nodeID string) (*models.NodeMastery, error) {
	nodesCollection := GetNodesCollection()

	// Get the folder node
	var node models.Node
	err := nodesCollection.FindOne(ctx, bson.M{"_id": nodeID}).Decode(&node)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrNodeNotFound
		}
		return nil, fmt.Errorf("failed to fetch node: %w", err)
	}

	if node.Metadata.Type != models.NodeTypeFolder {
		return nil, ErrInvalidNodeType
	}

	// Get all child nodes
	cursor, err := nodesCollection.Find(ctx, bson.M{"parentId": nodeID, "ownerId": node.OwnerID})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch children: %w", err)
	}
	defer cursor.Close(ctx)

	var children []models.Node
	if err = cursor.All(ctx, &children); err != nil {
		return nil, fmt.Errorf("failed to decode children: %w", err)
	}

	// Aggregate mastery from all children
	var (
		totalNotes    int
		masteredNotes int
		learntNotes   int
		lastStudyDate time.Time
		hasStudyData  bool
	)

	for _, child := range children {
		childMastery := child.Mastery
		totalNotes += childMastery.TotalNotes
		masteredNotes += childMastery.MasteredNotes
		learntNotes += childMastery.LearntNotes

		if !childMastery.LastStudyDate.IsZero() && (childMastery.LastStudyDate.After(lastStudyDate) || lastStudyDate.IsZero()) {
			lastStudyDate = childMastery.LastStudyDate
			hasStudyData = true
		}
	}

	now := time.Now()
	masteryPercent := 0.0
	if totalNotes > 0 {
		masteryPercent = float64(masteredNotes) / float64(totalNotes)
	}

	mastery := &models.NodeMastery{
		TotalNotes:     totalNotes,
		MasteredNotes:  masteredNotes,
		LearntNotes:    learntNotes,
		MasteryLevel:   DetermineMasteryLevel(masteryPercent),
		MasteryPercent: masteryPercent,
		LastUpdated:    now,
	}

	if hasStudyData {
		mastery.LastStudyDate = lastStudyDate
	} else {
		mastery.LastStudyDate = now
	}

	return mastery, nil
}

// UpdateNodeMastery updates the mastery for a node
func UpdateNodeMastery(ctx context.Context, nodeID string) error {
	nodesCollection := GetNodesCollection()

	// Get the node
	var node models.Node
	err := nodesCollection.FindOne(ctx, bson.M{"_id": nodeID}).Decode(&node)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return ErrNodeNotFound
		}
		return fmt.Errorf("failed to fetch node: %w", err)
	}

	// Calculate mastery based on node type
	var mastery *models.NodeMastery
	if node.Metadata.Type == models.NodeTypeNote {
		mastery, err = CalculateNoteMastery(ctx, nodeID)
	} else {
		mastery, err = CalculateFolderMastery(ctx, nodeID)
	}

	if err != nil {
		return fmt.Errorf("failed to calculate mastery: %w", err)
	}

	// Update the node
	update := bson.M{
		"$set": bson.M{
			"mastery":   mastery,
			"updatedAt": time.Now(),
		},
	}

	_, err = nodesCollection.UpdateOne(ctx, bson.M{"_id": nodeID}, update)
	if err != nil {
		return fmt.Errorf("failed to update node mastery: %w", err)
	}

	return nil
}

// UpdateNoteReview updates a note's review and queues ancestor updates
func UpdateNoteReview(ctx context.Context, nodeID, userID string, isCorrect bool) error {
	quality := QualityAgain
	if isCorrect {
		quality = QualityGood
	}

	// Initialize or get existing review
	review, err := InitializeNoteReview(ctx, nodeID, userID)
	if err != nil {
		return fmt.Errorf("failed to initialize note review: %w", err)
	}

	// Calculate new values using SM-2 algorithm
	newEaseFactor, newInterval, newRepetitions := CalculateNextReview(
		review.EaseFactor,
		review.Interval,
		review.Repetitions,
		quality,
	)

	// Calculate next review date
	nextReview := time.Now().AddDate(0, 0, newInterval)

	// If wrong answer, mark for review immediately
	toReview := false
	if !isCorrect {
		toReview = true
		nextReview = time.Now()
	}

	// Update database
	update := bson.M{
		"$set": bson.M{
			"easeFactor":   newEaseFactor,
			"interval":     newInterval,
			"repetitions":  newRepetitions,
			"nextReview":   nextReview,
			"totalReviews": review.TotalReviews + 1,
			"toReview":     toReview,
			"updatedAt":    time.Now(),
		},
	}

	if isCorrect {
		update["$inc"] = bson.M{"correctCount": 1}
	}

	collection := GetNoteReviewsCollection()
	_, err = collection.UpdateOne(ctx, bson.M{"_id": review.ID}, update)
	if err != nil {
		return fmt.Errorf("failed to update note review: %w", err)
	}

	// Update the note node's mastery immediately
	if err = UpdateNodeMastery(ctx, nodeID); err != nil {
		log.Printf("Failed to update note mastery: %v", err)
	}

	return nil
}

// UpdateAncestorMasteryAsync updates mastery for all ancestor folders
// This is intended to be called from the background queue
func UpdateAncestorMasteryAsync(ctx context.Context, nodeID string) error {
	log.Printf("Updating ancestor mastery for node: %s", nodeID)

	nodesCollection := GetNodesCollection()

	// Get the node to find its parent
	var node models.Node
	err := nodesCollection.FindOne(ctx, bson.M{"_id": nodeID}).Decode(&node)
	if err != nil {
		return fmt.Errorf("failed to fetch node: %w", err)
	}

	// Traverse up the tree and update each ancestor
	currentParentID := node.ParentID
	for currentParentID != "" {
		// Get parent node
		var parentNode models.Node
		err = nodesCollection.FindOne(ctx, bson.M{"_id": currentParentID}).Decode(&parentNode)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				// Parent doesn't exist, stop traversal
				break
			}
			return fmt.Errorf("failed to fetch parent node: %w", err)
		}

		// Only update mastery for folders
		if parentNode.Metadata.Type == models.NodeTypeFolder {
			// Calculate and update mastery for this folder
			mastery, err := CalculateFolderMastery(ctx, currentParentID)
			if err != nil {
				log.Printf("Failed to calculate mastery for folder %s: %v", currentParentID, err)
				return fmt.Errorf("failed to calculate folder mastery: %w", err)
			}

			// Update the folder's mastery
			update := bson.M{
				"$set": bson.M{
					"mastery":   mastery,
					"updatedAt": time.Now(),
				},
			}

			_, err = nodesCollection.UpdateOne(ctx, bson.M{"_id": currentParentID}, update)
			if err != nil {
				log.Printf("Failed to update mastery for folder %s: %v", currentParentID, err)
				return fmt.Errorf("failed to update folder mastery: %w", err)
			}

			log.Printf("Updated mastery for folder %s: %d/%d mastered", currentParentID, mastery.MasteredNotes, mastery.TotalNotes)
		}

		// Move up to the next ancestor
		currentParentID = parentNode.ParentID
	}

	return nil
}

// GetNodesByParent returns all child nodes for a given parent
func GetNodesByParent(ctx context.Context, parentID, userID string) ([]models.Node, error) {
	nodesCollection := GetNodesCollection()

	filter := bson.M{
		"parentId": parentID,
		"_id":      bson.M{"$ne": parentID}, // Exclude self-referencing nodes
	}
	if userID != "" {
		filter["ownerId"] = userID
	}

	cursor, err := nodesCollection.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch child nodes: %w", err)
	}
	defer cursor.Close(ctx)

	var nodes []models.Node
	if err = cursor.All(ctx, &nodes); err != nil {
		return nil, fmt.Errorf("failed to decode child nodes: %w", err)
	}

	return nodes, nil
}

// GetNodeByID returns a node by its ID
func GetNodeByID(ctx context.Context, nodeID string) (*models.Node, error) {
	nodesCollection := GetNodesCollection()

	var node models.Node
	objectID, err := primitive.ObjectIDFromHex(nodeID)
	if err != nil {
		return nil, fmt.Errorf("invalid node ID format: %w", err)
	}
	err = nodesCollection.FindOne(ctx, bson.M{"_id": objectID}).Decode(&node)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrNodeNotFound
		}
		return nil, fmt.Errorf("failed to fetch node: %w", err)
	}

	return &node, nil
}

// GetNodeTree returns a node and all its descendants
func GetNodeTree(ctx context.Context, nodeID string, maxDepth int) (*models.Node, []models.Node, error) {
	root, err := GetNodeByID(ctx, nodeID)
	if err != nil {
		return nil, nil, err
	}

	if maxDepth <= 0 {
		return root, []models.Node{}, nil
	}

	// Get direct children
	children, err := GetNodesByParent(ctx, nodeID, root.OwnerID)
	if err != nil {
		return root, children, nil
	}

	// Recursively get descendants
	var allDescendants []models.Node
	allDescendants = append(allDescendants, children...)

	for _, child := range children {
		_, descendants, err := GetNodeTree(ctx, child.ID.Hex(), maxDepth-1)
		if err != nil {
			continue
		}
		allDescendants = append(allDescendants, descendants...)
	}

	return root, allDescendants, nil
}

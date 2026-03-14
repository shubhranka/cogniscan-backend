package services

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"cogniscan/backend/internal/database"
	"cogniscan/backend/internal/models"
)

type AnswerQuality int

const (
	QualityAgain AnswerQuality = 0 // Wrong answer or complete blackout
	QualityHard  AnswerQuality = 3 // Correct, but difficult
	QualityGood  AnswerQuality = 4 // Correct with hesitation
	QualityEasy  AnswerQuality = 5 // Perfect, quick response
)

// GetReviewCollection returns the note_reviews collection
func GetReviewCollection() *mongo.Collection {
	return database.Client.Database("cogniscan").Collection("note_reviews")
}

// SM-2 Algorithm: CalculateNextReview
func CalculateNextReview(easeFactor float32, interval int, repetitions int, quality AnswerQuality) (newEaseFactor float32, newInterval int, newRepetitions int) {
	// Update ease factor
	// EF' = EF + (0.1 - (5 - q) * (0.08 + (5 - q) * 0.02))
	q := float32(quality)
	newEaseFactor = easeFactor + float32(0.1-(5-q)*(0.08+(5-q)*0.02))
	if newEaseFactor < 1.3 {
		newEaseFactor = 1.3
	}

	// Update repetitions and interval
	newRepetitions = repetitions
	newInterval = interval

	if quality < QualityHard {
		// Answer was wrong - reset
		newRepetitions = 0
		newInterval = 1
	} else {
		// Correct answer
		newRepetitions++
		if newRepetitions == 1 {
			newInterval = 1
		} else if newRepetitions == 2 {
			newInterval = 6
		} else {
			// I(n) = I(n-1) * EF
			newInterval = int(float32(newInterval) * newEaseFactor)
		}
	}

	return newEaseFactor, newInterval, newRepetitions
}

// InitializeNoteReview creates a new review entry for a note
func InitializeNoteReview(ctx context.Context, noteID, userID string) (*models.NoteReview, error) {
	collection := GetReviewCollection()

	// Check if already exists
	var existing models.NoteReview
	err := collection.FindOne(ctx, bson.M{"noteId": noteID, "userId": userID}).Decode(&existing)
	if err == nil {
		return &existing, nil
	}

	now := time.Now()
	review := &models.NoteReview{
		NoteID:       noteID,
		UserID:       userID,
		EaseFactor:   2.5,
		Interval:     0,
		Repetitions:  0,
		NextReview:   now,
		TotalReviews: 0,
		CorrectCount: 0,
		ToReview:     false,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	result, err := collection.InsertOne(ctx, review)
	if err != nil {
		return nil, err
	}

	review.ID = result.InsertedID.(primitive.ObjectID)
	return review, nil
}

// ProcessQuestionAnswer updates review data for all referenced notes
func ProcessQuestionAnswer(ctx context.Context, question *models.Question, userID string, isCorrect bool, timeTaken int) error {
	// Map answer to quality
	quality := QualityAgain
	if isCorrect {
		// Simple mapping - could be enhanced with time-based adjustment
		quality = QualityGood
	}

	for _, noteID := range question.ReferencedNoteIDs {
		review, err := InitializeNoteReview(ctx, noteID, userID)
		if err != nil {
			continue
		}

		// Calculate new values
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

		collection := GetReviewCollection()
		_, err = collection.UpdateOne(
			ctx,
			bson.M{"_id": review.ID},
			update,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

// GetReviewQueue returns notes due for review
func GetReviewQueue(ctx context.Context, userID string, limit int) ([]models.NoteReview, error) {
	collection := GetReviewCollection()
	now := time.Now()

	filter := bson.M{
		"userId": userID,
		"$or": []bson.M{
			{"toReview": true},
			{"nextReview": bson.M{"$lte": now}},
		},
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "toReview", Value: -1}, {Key: "nextReview", Value: 1}}).
		SetLimit(int64(limit))

	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}

	var reviews []models.NoteReview
	if err := cursor.All(ctx, &reviews); err != nil {
		return nil, err
	}

	return reviews, nil
}

// GetNoteReviewHistory retrieves review data for a note
func GetNoteReviewHistory(ctx context.Context, noteID, userID string) (*models.NoteReview, error) {
	collection := GetReviewCollection()
	var review models.NoteReview
	err := collection.FindOne(ctx, bson.M{"noteId": noteID, "userId": userID}).Decode(&review)
	return &review, err
}

// UpdateReviewStatus clears the toReview flag and updates nextReview when a review is opened
func UpdateReviewStatus(ctx context.Context, noteID, userID string) error {
	collection := GetReviewCollection()

	// Get the existing review
	var review models.NoteReview
	err := collection.FindOne(ctx, bson.M{"noteId": noteID, "userId": userID}).Decode(&review)
	if err != nil {
		return err
	}

	// Calculate new values using SM-2 algorithm with QualityGood as default
	// QualityGood (4) is used since user is proactively reviewing the note
	newEaseFactor, newInterval, newRepetitions := CalculateNextReview(
		review.EaseFactor,
		review.Interval,
		review.Repetitions,
		QualityGood,
	)

	// Calculate next review date
	nextReview := time.Now().AddDate(0, 0, newInterval)

	// Update database
	update := bson.M{
		"$set": bson.M{
			"easeFactor":  newEaseFactor,
			"interval":    newInterval,
			"repetitions": newRepetitions,
			"nextReview":  nextReview,
			"toReview":    false,
			"updatedAt":   time.Now(),
		},
		"$inc": bson.M{
			"totalReviews": 1,
		},
	}

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": review.ID},
		update,
	)

	return err
}

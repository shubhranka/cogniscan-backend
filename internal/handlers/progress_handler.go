package handlers

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"cogniscan/backend/internal/cache"
	"cogniscan/backend/internal/database"
	"cogniscan/backend/internal/models"
)

// UserProgressResponse represents the user progress response
type UserProgressResponse struct {
	ID                 primitive.ObjectID `json:"id"`
	UserID             string             `json:"userId"`
	CurrentStreak      int                `json:"currentStreak"`
	LongestStreak      int                `json:"longestStreak"`
	LastActiveDate     time.Time          `json:"lastActiveDate"`
	DailyGoalPercent  int                `json:"dailyGoalPercent"`
	DailyGoalDate     time.Time          `json:"dailyGoalDate"`
	StorageUsedBytes  int64              `json:"storageUsedBytes"`
	StorageQuotaBytes int64              `json:"storageQuotaBytes"`
	SessionAccuracy   float64            `json:"sessionAccuracy"`
	SessionAvgSpeed  float64            `json:"sessionAvgSpeed"`
	SessionStreak    int                `json:"sessionStreak"`
	CreatedAt          time.Time          `json:"createdAt"`
	UpdatedAt          time.Time          `json:"updatedAt"`
}

// DailyProgressUpdateRequest represents a daily progress update request
type DailyProgressUpdateRequest struct {
	DailyGoalPercent int `json:"dailyGoalPercent" binding:"required"`
}

// StudySessionRequest represents a study session recording
type StudySessionRequest struct {
	MinutesSpent int `json:"minutesSpent" binding:"required"`
}

// GetCurrentUserProgress returns user progress statistics
// @Summary Returns comprehensive user progress including streaks, daily goals, storage usage, and session statistics
func GetCurrentUserProgress(c *gin.Context) {
	userID := c.GetString("userId")
	if userID == "" {
		c.JSON(400, gin.H{"error": "userId required"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db := database.Client.Database(os.Getenv("DB_NAME"))
	collection := db.Collection("user_progress")

	var progress models.UserProgress
	err := collection.FindOne(ctx, bson.M{"userId": userID}).Decode(&progress)
	if err != nil {
		// Return default progress if not found
		storageQuotaBytes := int64(10737418240) // 10GB default
		if quotaStr := os.Getenv("STORAGE_QUOTA_BYTES"); quotaStr != "" {
			if parsed, parseErr := parseInt64(quotaStr); parseErr == nil {
				storageQuotaBytes = parsed
			}
		}

		defaultProgress := &UserProgressResponse{
			UserID:           userID,
			CurrentStreak:     0,
			LongestStreak:     0,
			LastActiveDate:    time.Now(),
			DailyGoalPercent:  0,
			DailyGoalDate:     time.Now(),
			StorageUsedBytes:  0,
			StorageQuotaBytes: storageQuotaBytes,
			SessionAccuracy:   0.0,
			SessionAvgSpeed:  0.0,
			SessionStreak:     0,
		}
		c.JSON(200, defaultProgress)
		return
	}

	response := &UserProgressResponse{
		ID:                 progress.ID,
		UserID:             progress.UserID,
		CurrentStreak:      progress.CurrentStreak,
		LongestStreak:      progress.LongestStreak,
		LastActiveDate:     progress.LastActiveDate,
		DailyGoalPercent:   progress.DailyGoalPercent,
		DailyGoalDate:      progress.DailyGoalDate,
		StorageUsedBytes:   progress.StorageUsedBytes,
		StorageQuotaBytes:  progress.StorageQuotaBytes,
		SessionAccuracy:    progress.SessionAccuracy,
		SessionAvgSpeed:   progress.SessionAvgSpeed,
		SessionStreak:     progress.SessionStreak,
		CreatedAt:         progress.CreatedAt,
		UpdatedAt:         progress.UpdatedAt,
	}

	c.JSON(200, response)
}

func parseInt64(s string) (int64, error) {
	var result int64
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}

// UpdateDailyProgress updates daily goal progress
// @Summary Updates the user's daily goal percentage for cognitive retention tracking
func UpdateDailyProgress(c *gin.Context) {
	userID := c.GetString("userId")
	if userID == "" {
		c.JSON(400, gin.H{"error": "userId required"})
		return
	}

	var req DailyProgressUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db := database.Client.Database(os.Getenv("DB_NAME"))
	collection := db.Collection("user_progress")

	now := time.Now()
	update := bson.M{
		"$set": bson.M{
			"dailyGoalPercent": req.DailyGoalPercent,
			"dailyGoalDate":    now,
			"lastActiveDate":   now,
			"updatedAt":        now,
		},
	}

	opts := options.Update().SetUpsert(true)
	_, err := collection.UpdateOne(ctx, bson.M{"userId": userID}, update, opts)
	if err != nil {
		log.Printf("Failed to update daily progress: %v", err)
		c.JSON(500, gin.H{"error": "Failed to update progress"})
		return
	}

	// Update Redis cache
	cacheKey := fmt.Sprintf("progress:user:%s", userID)
	err = cache.SetCache(cacheKey, map[string]interface{}{
		"userId":          userID,
		"dailyGoalPercent": req.DailyGoalPercent,
		"dailyGoalDate":    now,
	}, 1*time.Hour)
	if err != nil {
		log.Printf("Failed to cache progress: %v", err)
	}

	c.JSON(200, gin.H{
		"message":   "Progress updated",
		"userId":    userID,
		"dailyGoal": req.DailyGoalPercent,
	})
}

// RecordStudySession records a study session for streak tracking
// @Summary Records a study session to track user activity and update streaks
func RecordStudySession(c *gin.Context) {
	userID := c.GetString("userId")
	if userID == "" {
		c.JSON(400, gin.H{"error": "userId required"})
		return
	}

	var req StudySessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db := database.Client.Database(os.Getenv("DB_NAME"))
	collection := db.Collection("user_progress")

	// Check if this is a new day (streak calculation)
	wasActiveToday, err := cache.CheckDailyActivity(userID)
	if err != nil {
		log.Printf("Failed to check daily activity: %v", err)
	}

	now := time.Now()

	// Fetch current progress to get existing streak
	var progress models.UserProgress
	err = collection.FindOne(ctx, bson.M{"userId": userID}).Decode(&progress)
	if err == nil {
		// Update streak based on activity
		currentStreak := progress.CurrentStreak
		longestStreak := progress.LongestStreak

		if !wasActiveToday {
			// New day - check if streak should continue
			lastActive := progress.LastActiveDate
			if lastActive.Add(48 * time.Hour).After(now) {
				// Within 48 hours, continue streak
				currentStreak++
			} else {
				// Streak broken, start fresh
				currentStreak = 1
			}
		}

		if currentStreak > longestStreak {
			longestStreak = currentStreak
		}

		update := bson.M{
			"$set": bson.M{
				"currentStreak":  currentStreak,
				"longestStreak":  longestStreak,
				"lastActiveDate": now,
				"updatedAt":      now,
			},
		}

		opts := options.Update().SetUpsert(true)
		collection.UpdateOne(ctx, bson.M{"userId": userID}, update, opts)

		// Update Redis streak
		cache.SetStreak(userID, currentStreak)
	}

	// Record daily activity
	cache.IncrementDaily(userID)
	cache.UpdateLastActiveDate(userID)

	c.JSON(200, gin.H{
		"message":       "Study session recorded",
		"userId":        userID,
		"minutesSpent": req.MinutesSpent,
	})
}

// GetStorageUsage returns storage usage statistics
// @Summary Returns the user's current storage usage and quota information
func GetStorageUsage(c *gin.Context) {
	userID := c.GetString("userId")
	if userID == "" {
		c.JSON(400, gin.H{"error": "userId required"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db := database.Client.Database(os.Getenv("DB_NAME"))

	// Get storage quota from user progress or default
	var progress models.UserProgress
	storageQuotaBytes := int64(10737418240) // 10GB default
	err := db.Collection("user_progress").FindOne(ctx, bson.M{"userId": userID}).Decode(&progress)
	if err == nil && progress.StorageQuotaBytes > 0 {
		storageQuotaBytes = progress.StorageQuotaBytes
	}

	// Calculate total storage from notes
	notesCollection := db.Collection("notes")
	pipeline := []bson.M{
		{"$match": bson.M{"ownerId": userID}},
		{"$count": "totalNotes"},
	}

	cursor, err := notesCollection.Aggregate(ctx, pipeline)
	if err != nil {
		log.Printf("Failed to count notes: %v", err)
	}

	var result struct {
		TotalNotes int `bson:"totalNotes"`
	}
	if cursor.Next(ctx) {
		cursor.Decode(&result)
	}
	cursor.Close(ctx)

	// Estimate storage (assuming average 5MB per note)
	// In production, you would track actual file sizes
	const avgNoteSizeMB = 5
	usedBytes := int64(result.TotalNotes * avgNoteSizeMB * 1024 * 1024)

	// Update user progress with actual storage used
	update := bson.M{
		"$set": bson.M{
			"storageUsedBytes": usedBytes,
			"updatedAt":        time.Now(),
		},
	}
	db.Collection("user_progress").UpdateOne(ctx, bson.M{"userId": userID}, update)

	usedGB := float64(usedBytes) / (1024 * 1024 * 1024)
	quotaGB := float64(storageQuotaBytes) / (1024 * 1024 * 1024)
	percentage := 0.0
	if storageQuotaBytes > 0 {
		percentage = (float64(usedBytes) / float64(storageQuotaBytes)) * 100
	}

	storage := gin.H{
		"usedBytes":  usedBytes,
		"quotaBytes": storageQuotaBytes,
		"usedGB":     usedGB,
		"quotaGB":    quotaGB,
		"percentage":  percentage,
	}

	c.JSON(200, storage)
}

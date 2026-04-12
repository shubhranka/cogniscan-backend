package handlers

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"cogniscan/backend/internal/cache"
	"cogniscan/backend/internal/database"
	"cogniscan/backend/internal/models"
)

// QuizSessionResponse represents the quiz session response
type QuizSessionResponse struct {
	ID             primitive.ObjectID `json:"id"`
	UserID         string             `json:"userId"`
	QuizID         string             `json:"quizId"`
	FolderID       string             `json:"folderId"`
	Status         string    `json:"status"`
	CurrentIndex   int       `json:"currentIndex"`
	TotalAnswered  int       `json:"totalAnswered"`
	CorrectAnswers int       `json:"correctAnswers"`
	TotalTimeSecs  int       `json:"totalTimeSecs"`
	CurrentStreak  int       `json:"currentStreak"`
	LongestStreak  int       `json:"longestStreak"`
	StartedAt      time.Time          `json:"startedAt"`
	CompletedAt    time.Time          `json:"completedAt,omitempty"`
}

// StartSessionRequest represents a quiz session start request
type StartSessionRequest struct {
	QuizID   string `json:"quizId" binding:"required"`
	FolderID string `json:"folderId" binding:"required"`
}

// AnswerRequest represents an answer submission request
type AnswerRequest struct {
	QuestionID     string `json:"questionId" binding:"required"`
	SelectedOption int    `json:"selectedOption" binding:"required"`
	TimeTaken      int    `json:"timeTaken"` // seconds to answer
}

// SessionUpdate represents a session progress update
type SessionUpdate struct {
	TotalAnswered  int  `json:"totalAnswered"`
	CorrectAnswers int  `json:"correctAnswers"`
	TotalTimeSecs  int  `json:"totalTimeSecs"`
	CurrentStreak  int  `json:"currentStreak"`
}

// StartQuizSession starts a new quiz session
// @Summary Initializes a new quiz session with tracking for accuracy, speed, and streak
func StartQuizSession(c *gin.Context) {
	userID := c.GetString("userId")
	if userID == "" {
		c.JSON(400, gin.H{"error": "userId required"})
		return
	}

	var req StartSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db := database.Client.Database(os.Getenv("DB_NAME"))
	collection := db.Collection("quiz_sessions")

	session := &models.QuizSession{
		UserID:       userID,
		QuizID:       req.QuizID,
		FolderID:     req.FolderID,
		Status:       "active",
		CurrentIndex: 0,
		StartedAt:    time.Now(),
	}

	result, err := collection.InsertOne(ctx, session)
	if err != nil {
		log.Printf("Failed to create quiz session: %v", err)
		c.JSON(500, gin.H{"error": "Failed to create quiz session"})
		return
	}

	session.ID = result.InsertedID.(primitive.ObjectID)

	// Store in Redis for active session tracking
	sessionData := map[string]interface{}{
		"sessionId":     session.ID.Hex(),
		"userId":        session.UserID,
		"quizId":        session.QuizID,
		"folderId":      session.FolderID,
		"status":        session.Status,
		"currentIndex":  session.CurrentIndex,
		"startedAt":     session.StartedAt,
	}
	err = cache.SetActiveSession(userID, session.ID.Hex(), sessionData)
	if err != nil {
		log.Printf("Failed to cache session: %v", err)
	}

	response := &QuizSessionResponse{
		ID:             session.ID,
		UserID:         session.UserID,
		QuizID:         session.QuizID,
		FolderID:       session.FolderID,
		Status:         session.Status,
		CurrentIndex:   session.CurrentIndex,
		TotalAnswered:  session.TotalAnswered,
		CorrectAnswers: session.CorrectAnswers,
		TotalTimeSecs:  session.TotalTimeSecs,
		CurrentStreak:  session.CurrentStreak,
		LongestStreak:  session.LongestStreak,
		StartedAt:      session.StartedAt,
		CompletedAt:    session.CompletedAt,
	}

	c.JSON(200, response)
}

// UpdateSessionProgress updates live session statistics
// @Summary Updates session stats in real-time as user answers questions
func UpdateSessionProgress(c *gin.Context) {
	userID := c.GetString("userId")
	sessionID := c.Param("sessionId")
	if userID == "" || sessionID == "" {
		c.JSON(400, gin.H{"error": "userId and sessionId required"})
		return
	}

	var req AnswerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db := database.Client.Database(os.Getenv("DB_NAME"))

	// Fetch the question to determine correct answer
	questionsCollection := db.Collection("questions")
	var question models.Question
	objectID, err := primitive.ObjectIDFromHex(req.QuestionID)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid question ID"})
		return
	}
	err = questionsCollection.FindOne(ctx, bson.M{"_id": objectID}).Decode(&question)
	if err != nil {
		c.JSON(404, gin.H{"error": "Question not found"})
		return
	}

	isCorrect := req.SelectedOption == question.CorrectOption
	timeTaken := req.TimeTaken
	if timeTaken == 0 {
		timeTaken = 60 // Default to 60 seconds if not provided
	}

	// Store QuestionAnswer
	answersCollection := db.Collection("question_answers")
	answer := &models.QuestionAnswer{
		QuestionID:     req.QuestionID,
		UserID:         userID,
		SelectedOption: req.SelectedOption,
		IsCorrect:      isCorrect,
		TimeTaken:      timeTaken,
		AnsweredAt:     time.Now(),
	}
	_, err = answersCollection.InsertOne(ctx, answer)
	if err != nil {
		log.Printf("Failed to store question answer: %v", err)
	}

	// Fetch current session
	sessionsCollection := db.Collection("quiz_sessions")
	var session models.QuizSession
	sessionObjID, err := primitive.ObjectIDFromHex(sessionID)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid session ID"})
		return
	}
	err = sessionsCollection.FindOne(ctx, bson.M{"_id": sessionObjID}).Decode(&session)
	if err != nil {
		c.JSON(404, gin.H{"error": "Session not found"})
		return
	}

	// Calculate new streak
	newCurrentStreak := session.CurrentStreak
	newLongestStreak := session.LongestStreak
	if isCorrect {
		newCurrentStreak++
		if newCurrentStreak > newLongestStreak {
			newLongestStreak = newCurrentStreak
		}
	} else {
		newCurrentStreak = 0
	}

	// Update session aggregate stats
	update := bson.M{
		"$set": bson.M{
			"totalAnswered":  session.TotalAnswered + 1,
			"correctAnswers": session.CorrectAnswers + boolToInt(isCorrect),
			"totalTimeSecs":  session.TotalTimeSecs + timeTaken,
			"currentStreak":  newCurrentStreak,
			"longestStreak":  newLongestStreak,
			"updatedAt":      time.Now(),
		},
	}

	_, err = sessionsCollection.UpdateOne(ctx, bson.M{"_id": sessionObjID}, update)
	if err != nil {
		log.Printf("Failed to update session: %v", err)
	}

	// Update Redis cache
	sessionData := map[string]interface{}{
		"sessionId":      sessionID,
		"userId":         userID,
		"totalAnswered":  session.TotalAnswered + 1,
		"correctAnswers": session.CorrectAnswers + boolToInt(isCorrect),
		"currentStreak":  newCurrentStreak,
	}
	cache.SetActiveSession(userID, sessionID, sessionData)

	c.JSON(200, gin.H{
		"message":     "Session progress updated",
		"sessionId":   sessionID,
		"questionId":  req.QuestionID,
		"isCorrect":  isCorrect,
	})
}

// CompleteQuizSession finalizes session
// @Summary Marks a session as completed and calculates final statistics
func CompleteQuizSession(c *gin.Context) {
	userID := c.GetString("userId")
	sessionID := c.Param("sessionId")
	if userID == "" || sessionID == "" {
		c.JSON(400, gin.H{"error": "userId and sessionId required"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db := database.Client.Database(os.Getenv("DB_NAME"))
	collection := db.Collection("quiz_sessions")

	sessionObjID, err := primitive.ObjectIDFromHex(sessionID)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid session ID"})
		return
	}

	// Update QuizSession status to "completed" and set completed timestamp
	now := time.Now()
	update := bson.M{
		"$set": bson.M{
			"status":      "completed",
			"completedAt":  now,
			"updatedAt":    now,
		},
	}

	_, err = collection.UpdateOne(ctx, bson.M{"_id": sessionObjID, "userId": userID}, update)
	if err != nil {
		log.Printf("Failed to complete session: %v", err)
		c.JSON(500, gin.H{"error": "Failed to complete session"})
		return
	}

	// Clear active session from Redis
	err = cache.ClearActiveSession(userID)
	if err != nil {
		log.Printf("Failed to clear session from cache: %v", err)
	}

	c.JSON(200, gin.H{
		"message":     "Session completed",
		"sessionId":   sessionID,
		"status":     "completed",
		"completedAt": now,
	})
}

// GetActiveSession returns active session for user
// @Summary Returns the currently active quiz session if one exists
func GetActiveSession(c *gin.Context) {
	userID := c.GetString("userId")
	if userID == "" {
		c.JSON(400, gin.H{"error": "userId required"})
		return
	}

	// Fetch from Redis
	sessionData, err := cache.GetActiveSession(userID)
	if err != nil {
		c.JSON(200, gin.H{
			"session": nil,
		})
		return
	}

	c.JSON(200, gin.H{
		"session": sessionData,
	})
}

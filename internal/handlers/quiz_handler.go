package handlers

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"cogniscan/backend/internal/cache"
	"cogniscan/backend/internal/middleware"
	"cogniscan/backend/internal/models"
	"cogniscan/backend/internal/queue"
	"cogniscan/backend/internal/services"
)

type CreateQuizResponse struct {
	Quiz      *models.Quiz      `json:"quiz"`
	Questions []models.Question `json:"questions"`
}

// SessionData represents a live quiz session with tracking
type SessionData struct {
	SessionID      string    `json:"sessionId"`
	UserID         string    `json:"userId"`
	QuizID         string    `json:"quizId"`
	FolderID       string    `json:"folderId"`
	StartedAt      time.Time `json:"startedAt"`
	TotalQuestions int       `json:"totalQuestions"`
}

// SessionStatistics represents live statistics for a quiz session
type SessionStatistics struct {
	SessionID     string  `json:"sessionId"`
	TotalAnswered int     `json:"totalAnswered"`
	CorrectCount  int     `json:"correctCount"`
	Accuracy      float64 `json:"accuracy"`
	AverageSpeed  float64 `json:"averageSpeed"` // seconds per question
	CurrentStreak int     `json:"currentStreak"`
	BestStreak    int     `json:"bestStreak"`
	IsNeuralMode  bool    `json:"isNeuralMode"`
	AdaptiveLevel string  `json:"adaptiveLevel"` // "beginner", "intermediate", "advanced", "expert"
}

// SubmitAnswerWithSessionPayload extends SubmitAnswerPayload with session tracking
type SubmitAnswerWithSessionPayload struct {
	SelectedOption int    `json:"selectedOption"`
	TimeTaken      int    `json:"timeTaken"`    // seconds, optional
	SessionID      string `json:"sessionId"`    // optional, for session tracking
	IsNeuralMode   bool   `json:"isNeuralMode"` // optional, indicates Neural Assessment Mode
}

// CreateQuiz generates a quiz for a folder (synchronous - for backward compatibility)
func CreateQuiz(c *gin.Context) {
	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	folderID := c.Param("folderId")

	quiz, questions, err := services.CreateQuizForFolder(c.Request.Context(), folderID, firebaseUser.Claims["email"].(string), false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Printf("Quiz: %+v, Questions: %+v", quiz, questions)

	c.JSON(http.StatusCreated, CreateQuizResponse{
		Quiz:      quiz,
		Questions: questions,
	})
}

// RequestQuizGeneration starts asynchronous quiz generation
func RequestQuizGeneration(c *gin.Context) {
	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	folderID := c.Param("folderId")

	// Check if a quiz is already being generated
	status, err := services.GetFolderQuizStatus(c.Request.Context(), folderID, firebaseUser.Claims["email"].(string))
	if err == nil && (status.Status == models.QuizGenStatusPending || status.Status == models.QuizGenStatusProcessing) {
		c.JSON(http.StatusConflict, gin.H{"error": "Quiz generation already in progress"})
		return
	}

	// Check if there's already a completed quiz
	if err == nil && status.Status == models.QuizGenStatusCompleted && status.QuizID != "" {
		// Quiz already exists, return its ID
		c.JSON(http.StatusOK, gin.H{
			"status":  "completed",
			"quizId":  status.QuizID,
			"message": "Quiz already exists for this folder",
		})
		return
	}

	// Create job
	jobID := uuid.New().String()
	job := queue.QuizJob{
		ID:       jobID,
		FolderID: folderID,
		OwnerID:  firebaseUser.Claims["email"].(string),
	}

	// Enqueue job
	if err := services.EnqueueQuizJob(job); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to queue quiz generation"})
		return
	}

	// Update folder status to pending
	if err := services.UpdateFolderQuizStatus(c.Request.Context(), folderID, firebaseUser.Claims["email"].(string), models.QuizGenStatusPending, "", ""); err != nil {
		log.Printf("Failed to update folder quiz status: %v", err)
	}

	c.JSON(http.StatusAccepted, gin.H{
		"status":  "queued",
		"jobId":   jobID,
		"message": "Quiz generation started",
	})
}

// GetQuizStatus gets the current quiz generation status for a folder
func GetQuizStatus(c *gin.Context) {
	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	folderID := c.Param("folderId")

	status, err := services.GetFolderQuizStatus(c.Request.Context(), folderID, firebaseUser.Claims["email"].(string))
	if err != nil {
		// If folder not found or no status, return default
		c.JSON(http.StatusOK, gin.H{
			"status": "none",
			"quizId": "",
		})
		return
	}

	c.JSON(http.StatusOK, status)
}

// GetQuiz retrieves quiz details
func GetQuiz(c *gin.Context) {
	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	quizID := c.Param("quizId")

	quiz, err := services.GetQuiz(c.Request.Context(), quizID, firebaseUser.Claims["email"].(string))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Quiz not found"})
		return
	}

	c.JSON(http.StatusOK, quiz)
}

// GetQuizQuestions retrieves all questions for a quiz
func GetQuizQuestions(c *gin.Context) {
	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	quizID := c.Param("quizId")

	// Verify quiz ownership
	quiz, err := services.GetQuiz(c.Request.Context(), quizID, firebaseUser.Claims["email"].(string))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Quiz not found"})
		return
	}

	questions, err := services.GetQuizQuestions(c.Request.Context(), quiz.ID.Hex())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get questions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"questions": questions,
		"total":     len(questions),
	})
}

type SubmitAnswerPayload struct {
	SelectedOption int `json:"selectedOption"`
	TimeTaken      int `json:"timeTaken"` // seconds, optional
}

type AnswerResponse struct {
	IsCorrect        bool               `json:"isCorrect"`
	Explanation      string             `json:"explanation"`
	CorrectOption    int                `json:"correctOption"`
	SessionStats     *SessionStatistics `json:"sessionStats,omitempty"`     // Live session statistics if session tracking enabled
	AdaptiveFeedback string             `json:"adaptiveFeedback,omitempty"` // AI-generated adaptive feedback in Neural Assessment Mode
}

// SubmitAnswer records an answer to a question with optional session tracking for Neural Assessment Mode
func SubmitAnswer(c *gin.Context) {
	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	quizID := c.Param("quizId")
	questionID := c.Param("questionId")

	// Try to bind as enhanced payload first (session tracking)
	var enhancedPayload SubmitAnswerWithSessionPayload
	var payload SubmitAnswerPayload
	useEnhanced := false

	if err := c.ShouldBindJSON(&enhancedPayload); err == nil && enhancedPayload.SessionID != "" {
		// Using enhanced payload with session tracking
		useEnhanced = true
		payload.SelectedOption = enhancedPayload.SelectedOption
		payload.TimeTaken = enhancedPayload.TimeTaken
	} else {
		// Fallback to basic payload (backward compatibility)
		if err := c.ShouldBindJSON(&payload); err != nil {
			log.Printf("Failed to bind JSON: %v, Content-Type: %s, Content-Length: %s", err,
				c.GetHeader("Content-Type"), c.GetHeader("Content-Length"))
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload", "details": err.Error()})
			return
		}
	}

	// Get question
	question, err := services.GetQuestion(c.Request.Context(), questionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Question not found"})
		return
	}

	// Check if question belongs to user's quiz
	quiz, err := services.GetQuiz(c.Request.Context(), quizID, firebaseUser.Claims["email"].(string))
	if err != nil || question.QuizID != quiz.ID.Hex() {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Check answer
	isCorrect := payload.SelectedOption == question.CorrectOption

	// Check if user already answered this question BEFORE inserting
	answerCollection := services.GetAnswerCollection()
	var existingAnswer bson.M
	checkErr := answerCollection.FindOne(
		c.Request.Context(),
		bson.M{
			"questionId": questionID,
			"userId":     firebaseUser.Claims["email"].(string),
		},
	).Decode(&existingAnswer)

	isFirstAnswer := checkErr == mongo.ErrNoDocuments

	// Save answer
	answer := &models.QuestionAnswer{
		QuestionID:     questionID,
		UserID:         firebaseUser.Claims["email"].(string),
		SelectedOption: payload.SelectedOption,
		IsCorrect:      isCorrect,
		TimeTaken:      payload.TimeTaken,
		AnsweredAt:     time.Now(),
	}

	collection := services.GetAnswerCollection()
	if _, err := collection.InsertOne(c.Request.Context(), answer); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save answer"})
		return
	}

	// Update review data for referenced notes
	if err := services.ProcessQuestionAnswer(c.Request.Context(), question, firebaseUser.Claims["email"].(string), isCorrect, payload.TimeTaken); err != nil {
		// Log error but don't fail the request
		// Review data is secondary
	}

	// ============ SESSION TRACKING (Neural Assessment Mode) ============
	var sessionStats *SessionStatistics
	var adaptiveFeedback string
	var cumulativeTotalTime int

	if useEnhanced {
		// Get existing session data from Redis
		existingSession, err := cache.GetActiveSession(firebaseUser.Claims["email"].(string))
		if err == nil && existingSession != nil {
			// Get existing cumulative time
			existingTotalTime := getInt(existingSession, "totalTime", 0)
			// Update session statistics
			sessionStats = updateSessionStatistics(existingSession, isCorrect, payload.TimeTaken, quiz.TotalQuestions, enhancedPayload.IsNeuralMode, existingTotalTime)
			cumulativeTotalTime = existingTotalTime + payload.TimeTaken
		} else {
			// Create new session statistics
			sessionStats = &SessionStatistics{
				SessionID:     enhancedPayload.SessionID,
				TotalAnswered: 1,
				CorrectCount:  boolToInt(isCorrect),
				Accuracy:      boolToFloat(isCorrect),
				AverageSpeed:  float64(payload.TimeTaken),
				CurrentStreak: boolToInt(isCorrect),
				BestStreak:    boolToInt(isCorrect),
				IsNeuralMode:  enhancedPayload.IsNeuralMode,
				AdaptiveLevel: "intermediate", // Default level
			}
			cumulativeTotalTime = payload.TimeTaken
		}

		// Store updated session data in Redis
		sessionData := map[string]interface{}{
			"sessionId":     sessionStats.SessionID,
			"userId":        firebaseUser.Claims["email"].(string),
			"quizId":        quizID,
			"folderId":      quiz.FolderID,
			"totalAnswered": sessionStats.TotalAnswered,
			"correctCount":  sessionStats.CorrectCount,
			"accuracy":      sessionStats.Accuracy,
			"averageSpeed":  sessionStats.AverageSpeed,
			"currentStreak": sessionStats.CurrentStreak,
			"bestStreak":    sessionStats.BestStreak,
			"isNeuralMode":  sessionStats.IsNeuralMode,
			"totalTime":     cumulativeTotalTime,
		}
		if err := cache.SetActiveSession(firebaseUser.Claims["email"].(string), enhancedPayload.SessionID, sessionData); err != nil {
			log.Printf("Failed to store session data in Redis: %v", err)
		}

		// Generate adaptive feedback if in Neural Assessment Mode
		if enhancedPayload.IsNeuralMode {
			adaptiveFeedback = generateAdaptiveFeedback(isCorrect, sessionStats)
		}
	}

	// Update quiz correct count only if this is the first time answering this question
	if isCorrect && isFirstAnswer {
		quizzesCollection := services.GetQuizCollection()
		result, err := quizzesCollection.UpdateOne(
			c.Request.Context(),
			bson.M{"_id": quiz.ID},
			bson.M{"$inc": bson.M{"correctAnswers": 1}},
		)
		if err != nil {
			log.Printf("Failed to update quiz correct count: %v", err)
		} else {
			log.Printf("Updated quiz correct count: matched %d, modified %d", result.MatchedCount, result.ModifiedCount)
		}
	} else {
		log.Printf("Not updating quiz count - isCorrect: %v, isFirstAnswer: %v", isCorrect, isFirstAnswer)
	}

	c.JSON(http.StatusOK, AnswerResponse{
		IsCorrect:        isCorrect,
		Explanation:      question.Explanation,
		CorrectOption:    question.CorrectOption,
		SessionStats:     sessionStats,
		AdaptiveFeedback: adaptiveFeedback,
	})
}

// GetQuizSummary returns quiz completion summary
func GetQuizSummary(c *gin.Context) {
	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	quizID := c.Param("quizId")

	quiz, err := services.GetQuiz(c.Request.Context(), quizID, firebaseUser.Claims["email"].(string))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Quiz not found"})
		return
	}

	log.Printf("Quiz summary - QuizID: %s, TotalQuestions: %d, CorrectAnswers: %d", quizID, quiz.TotalQuestions, quiz.CorrectAnswers)

	completionRate := 0.0
	if quiz.TotalQuestions > 0 {
		completionRate = float64(quiz.CorrectAnswers) / float64(quiz.TotalQuestions) * 100
	}

	c.JSON(http.StatusOK, gin.H{
		"totalQuestions": quiz.TotalQuestions,
		"correctAnswers": quiz.CorrectAnswers,
		"completionRate": completionRate,
		"status":         quiz.Status,
	})
}

// RegenerateQuiz deletes the existing quiz and triggers regeneration for a folder
func RegenerateQuiz(c *gin.Context) {
	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	quizID := c.Param("quizId")

	// Get quiz to find folder ID
	quiz, err := services.GetQuiz(c.Request.Context(), quizID, firebaseUser.Claims["email"].(string))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Quiz not found"})
		return
	}

	folderID := quiz.FolderID

	// Get all question IDs for this quiz to delete their answers
	questionsCollection := services.GetQuestionCollection()
	cursor, err := questionsCollection.Find(c.Request.Context(), bson.M{"quizId": quizID})
	if err != nil {
		log.Printf("Failed to find quiz questions: %v", err)
	} else {
		var questionIDs []string
		for cursor.Next(c.Request.Context()) {
			var question models.Question
			if err := cursor.Decode(&question); err == nil {
				questionIDs = append(questionIDs, question.ID.Hex())
			}
		}
		cursor.Close(c.Request.Context())

		// Delete existing question answers by question IDs
		if len(questionIDs) > 0 {
			answerCollection := services.GetAnswerCollection()
			answerCollection.DeleteMany(c.Request.Context(), bson.M{
				"questionId": bson.M{"$in": questionIDs},
				"userId":     firebaseUser.Claims["email"].(string),
			})
		}
	}

	// Delete existing quiz questions
	questionsCollection.DeleteMany(c.Request.Context(), bson.M{"quizId": quizID})

	// Delete the quiz document
	quizzesCollection := services.GetQuizCollection()
	quizzesCollection.DeleteOne(c.Request.Context(), bson.M{"_id": quiz.ID, "ownerId": firebaseUser.Claims["email"].(string)})

	// Create job for regeneration
	jobID := uuid.New().String()
	job := queue.QuizJob{
		ID:       jobID,
		FolderID: folderID,
		OwnerID:  firebaseUser.Claims["email"].(string),
	}

	// Enqueue job
	if err := services.EnqueueQuizJob(job); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to queue quiz regeneration"})
		return
	}

	// Update folder status to pending (triggers regeneration)
	if err := services.UpdateFolderQuizStatus(
		c.Request.Context(),
		folderID,
		firebaseUser.Claims["email"].(string),
		models.QuizGenStatusPending,
		"",
		"",
	); err != nil {
		log.Printf("Failed to update folder quiz status: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Quiz regeneration started",
		"folderId": folderID,
		"jobId":    jobID,
	})
}

// ============ SESSION TRACKING HELPER FUNCTIONS ============

// updateSessionStatistics updates the session statistics based on a new answer
func updateSessionStatistics(existingSession map[string]interface{}, isCorrect bool, timeTaken int, totalQuestions int, isNeuralMode bool, existingTotalTime int) *SessionStatistics {
	totalAnswered := getInt(existingSession, "totalAnswered", 0) + 1
	correctCount := getInt(existingSession, "correctCount", 0)
	currentStreak := getInt(existingSession, "currentStreak", 0)
	bestStreak := getInt(existingSession, "bestStreak", 0)

	// Use the existing cumulative time provided from Redis
	totalTime := float64(existingTotalTime) + float64(timeTaken)

	if isCorrect {
		correctCount++
		currentStreak++
		if currentStreak > bestStreak {
			bestStreak = currentStreak
		}
	} else {
		currentStreak = 0
	}

	accuracy := float64(correctCount) / float64(totalAnswered) * 100
	averageSpeed := totalTime / float64(totalAnswered)

	adaptiveLevel := "intermediate"
	if isNeuralMode {
		adaptiveLevel = calculateAdaptiveLevel(accuracy, averageSpeed)
	}

	return &SessionStatistics{
		SessionID:     getString(existingSession, "sessionId", ""),
		TotalAnswered: totalAnswered,
		CorrectCount:  correctCount,
		Accuracy:      accuracy,
		AverageSpeed:  averageSpeed,
		CurrentStreak: currentStreak,
		BestStreak:    bestStreak,
		IsNeuralMode:  isNeuralMode,
		AdaptiveLevel: adaptiveLevel,
	}
}

// generateAdaptiveFeedback generates adaptive feedback based on current performance
func generateAdaptiveFeedback(isCorrect bool, stats *SessionStatistics) string {
	if isCorrect {
		if stats.CurrentStreak >= 5 {
			return fmt.Sprintf("Excellent! You're on a %d-question streak. Your accuracy of %.1f%% shows strong mastery. The AI is preparing more challenging questions.", stats.CurrentStreak, stats.Accuracy)
		} else if stats.Accuracy >= 80 {
			return "Well done! Your strong performance suggests you're ready for more advanced concepts."
		}
		return "Correct! Keep building on this momentum."
	} else {
		if stats.CurrentStreak == 0 && stats.Accuracy < 50 {
			return "Let's focus on understanding the fundamentals. Review the explanation and try similar questions."
		} else if stats.Accuracy >= 70 {
			return "Nice attempt! You're showing good progress overall. This concept needs a bit more reinforcement."
		}
		return "Not quite right. Review the explanation and consider reviewing related notes in this folder."
	}
}

// calculateAdaptiveLevel determines the adaptive difficulty level based on performance
func calculateAdaptiveLevel(accuracy float64, averageSpeed float64) string {
	// Accuracy is primary factor, speed is secondary
	if accuracy >= 90 && averageSpeed < 15 {
		return "expert"
	} else if accuracy >= 80 || (accuracy >= 70 && averageSpeed < 10) {
		return "advanced"
	} else if accuracy >= 60 {
		return "intermediate"
	}
	return "beginner"
}

// Helper functions for extracting values from session map
func getInt(m map[string]interface{}, key string, defaultValue int) int {
	if val, ok := m[key]; ok {
		if intVal, ok := val.(int); ok {
			return intVal
		}
		if floatVal, ok := val.(float64); ok {
			return int(floatVal)
		}
	}
	return defaultValue
}

func getString(m map[string]interface{}, key string, defaultValue string) string {
	if val, ok := m[key]; ok {
		if strVal, ok := val.(string); ok {
			return strVal
		}
	}
	return defaultValue
}

// Helper functions for boolean conversion
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func boolToFloat(b bool) float64 {
	if b {
		return 100.0
	}
	return 0.0
}

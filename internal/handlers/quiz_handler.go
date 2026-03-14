package handlers

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"cogniscan/backend/internal/middleware"
	"cogniscan/backend/internal/models"
	"cogniscan/backend/internal/queue"
	"cogniscan/backend/internal/services"
)

type CreateQuizResponse struct {
	Quiz      *models.Quiz      `json:"quiz"`
	Questions []models.Question `json:"questions"`
}

// CreateQuiz generates a quiz for a folder (synchronous - for backward compatibility)
func CreateQuiz(c *gin.Context) {
	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	folderID := c.Param("folderId")

	quiz, questions, err := services.CreateQuizForFolder(c.Request.Context(), folderID, firebaseUser.UID, false)
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
	status, err := services.GetFolderQuizStatus(c.Request.Context(), folderID, firebaseUser.UID)
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
		OwnerID:  firebaseUser.UID,
	}

	// Enqueue job
	if err := services.EnqueueQuizJob(job); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to queue quiz generation"})
		return
	}

	// Update folder status to pending
	if err := services.UpdateFolderQuizStatus(c.Request.Context(), folderID, firebaseUser.UID, models.QuizGenStatusPending, "", ""); err != nil {
		log.Printf("Failed to update folder quiz status: %v", err)
	}

	c.JSON(http.StatusAccepted, gin.H{
		"status":   "queued",
		"jobId":    jobID,
		"message":  "Quiz generation started",
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

	status, err := services.GetFolderQuizStatus(c.Request.Context(), folderID, firebaseUser.UID)
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

	quiz, err := services.GetQuiz(c.Request.Context(), quizID, firebaseUser.UID)
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
	quiz, err := services.GetQuiz(c.Request.Context(), quizID, firebaseUser.UID)
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
	IsCorrect     bool   `json:"isCorrect"`
	Explanation   string `json:"explanation"`
	CorrectOption int    `json:"correctOption"`
}

// SubmitAnswer records an answer to a question
func SubmitAnswer(c *gin.Context) {
	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	quizID := c.Param("quizId")
	questionID := c.Param("questionId")

	var payload SubmitAnswerPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		log.Printf("Failed to bind JSON: %v, Content-Type: %s, Content-Length: %s", err,
			c.GetHeader("Content-Type"), c.GetHeader("Content-Length"))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload", "details": err.Error()})
		return
	}

	// Get question
	question, err := services.GetQuestion(c.Request.Context(), questionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Question not found"})
		return
	}

	// Check if question belongs to user's quiz
	quiz, err := services.GetQuiz(c.Request.Context(), quizID, firebaseUser.UID)
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
			"userId":     firebaseUser.UID,
		},
	).Decode(&existingAnswer)

	isFirstAnswer := checkErr == mongo.ErrNoDocuments

	// Save answer
	answer := &models.QuestionAnswer{
		QuestionID:     questionID,
		UserID:         firebaseUser.UID,
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
	if err := services.ProcessQuestionAnswer(c.Request.Context(), question, firebaseUser.UID, isCorrect, payload.TimeTaken); err != nil {
		// Log error but don't fail the request
		// Review data is secondary
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
		IsCorrect:     isCorrect,
		Explanation:   question.Explanation,
		CorrectOption: question.CorrectOption,
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

	quiz, err := services.GetQuiz(c.Request.Context(), quizID, firebaseUser.UID)
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
	quiz, err := services.GetQuiz(c.Request.Context(), quizID, firebaseUser.UID)
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
				"userId":      firebaseUser.UID,
			})
		}
	}

	// Delete existing quiz questions
	questionsCollection.DeleteMany(c.Request.Context(), bson.M{"quizId": quizID})

	// Delete the quiz document
	quizzesCollection := services.GetQuizCollection()
	quizzesCollection.DeleteOne(c.Request.Context(), bson.M{"_id": quiz.ID, "ownerId": firebaseUser.UID})

	// Create job for regeneration
	jobID := uuid.New().String()
	job := queue.QuizJob{
		ID:       jobID,
		FolderID: folderID,
		OwnerID:  firebaseUser.UID,
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
		firebaseUser.UID,
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

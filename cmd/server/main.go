package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strconv"
	"strings"

	"cogniscan/backend/internal/cache"
	"cogniscan/backend/internal/database"
	"cogniscan/backend/internal/handlers"
	"cogniscan/backend/internal/middleware"
	"cogniscan/backend/internal/queue"
	"cogniscan/backend/internal/services"
	"cogniscan/backend/internal/workers"

	firebase "firebase.google.com/go/v4"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"google.golang.org/api/option"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, relying on environment variables")
	}

	// Initialize Database
	database.ConnectDB()

	if err := cache.InitRedis(); err != nil {
		log.Printf("Warning: Failed to initialize Redis: %v", err)
		// Continue without Redis - graceful degradation
	}

	// Check if token.json exists, if not create it
	if _, err := os.Stat("service-account.json"); os.IsNotExist(err) {
		token := os.Getenv("KEY_DATA_N")
		if token == "" {
			log.Println("KEY_DATA_N environment variable not set")
			return
		}

		var tokenJson map[string]interface{}
		err = json.Unmarshal([]byte(token), &tokenJson)
		if err != nil {
			log.Fatalf("error parsing service-account.json: %v\n", err)
		}
		tokenJson["private_key"] = strings.ReplaceAll(tokenJson["private_key"].(string), "\\n", "\n")

		tokenBytes, err := json.Marshal(tokenJson)
		if err != nil {
			log.Fatalf("error marshaling service-account.json: %v\n", err)
		}

		err = os.WriteFile("service-account.json", tokenBytes, 0644)
		if err != nil {
			log.Fatalf("error writing service-account.json: %v\n", err)
		}
	}

	// Initialize Google Drive Service
	if err := services.InitDriveService(); err != nil {
		log.Fatalf("Failed to initialize Drive Service: %v", err)
	}

	// Initialize AI Service for caption generation
	if err := services.InitAIService(); err != nil {
		log.Printf("Warning: Failed to initialize AI Service: %v", err)
		// Continue without AI service - graceful degradation
	}

	// Initialize Vector Service for embedding storage
	if err := services.InitVectorService(); err != nil {
		log.Printf("Warning: Failed to initialize Vector Service: %v", err)
		// Continue without vector service - vector search will be disabled
	}

	// Initialize Queue Service for background workers
	if err := services.InitQueueService(); err != nil {
		log.Printf("Warning: Failed to initialize Queue Service: %v", err)
		// Continue without queue service - caption generation will be disabled
	}

	// Initialize Mastery Queue for background mastery propagation
	masteryWorkerCount := 2
	if mwc := os.Getenv("MASTERY_WORKER_COUNT"); mwc != "" {
		if n, err := strconv.Atoi(mwc); err == nil && n > 0 {
			masteryWorkerCount = n
		}
	}
	masteryQueue := queue.NewMasteryQueue(masteryWorkerCount, nil)
	services.SetMasteryQueue(masteryQueue)
	masteryQueue.Start()

	log.Printf("[Main] Mastery queue started with %d workers", masteryWorkerCount)

	// Initialize Firebase Admin SDK from Environment Variable
	mainCtx := context.Background()
	keyDataString := os.Getenv("KEY_DATA")
	if keyDataString == "" {
		log.Fatal("KEY_DATA environment variable not set")
	}
	var parsedKeyData map[string]interface{}
	err := json.Unmarshal([]byte(keyDataString), &parsedKeyData)
	if err != nil {
		log.Fatalf("error unmarshalling key data: %v\n", err)
	}
	parsedKeyData["private_key"] = strings.ReplaceAll(parsedKeyData["private_key"].(string), "\\n", "\n")
	parsedKeyDataString, err := json.Marshal(parsedKeyData)
	if err != nil {
		log.Fatalf("error marshalling key data: %v\n", err)
	}
	opt := option.WithCredentialsJSON(parsedKeyDataString)
	app, err := firebase.NewApp(mainCtx, nil, opt)
	if err != nil {
		log.Fatalf("error initializing app: %v\n", err)
	}
	authClient, err := app.Auth(mainCtx)
	if err != nil {
		log.Fatalf("error getting Auth client: %v\n", err)
	}

	// Start caption workers
	workerCount := 3
	if wc := os.Getenv("CAPTION_WORKER_COUNT"); wc != "" {
		if n, err := strconv.Atoi(wc); err == nil && n > 0 {
			workerCount = n
		}
	}
	workers.StartCaptionWorker(mainCtx, workerCount)

	log.Printf("[Main] Caption workers started with count: %d", workerCount)

	// Start quiz workers
	quizWorkerCount := 2
	if qwc := os.Getenv("QUIZ_WORKER_COUNT"); qwc != "" {
		if n, err := strconv.Atoi(qwc); err == nil && n > 0 {
			quizWorkerCount = n
		}
	}
	workers.StartQuizWorker(mainCtx, quizWorkerCount)

	log.Printf("[Main] Quiz workers started with count: %d", quizWorkerCount)

	// Initialize Gin Router
	router := gin.Default()
	router.GET("/health", handlers.HealthCheck)

	api := router.Group("/api/v1")
	{
		protected := api.Group("/").Use(middleware.AuthMiddleware(authClient))
		{
			// PROGRESS ROUTES
			protected.GET("/progress/:userId", handlers.GetCurrentUserProgress)
			protected.POST("/progress/daily", handlers.UpdateDailyProgress)
			protected.POST("/progress/study-session", handlers.RecordStudySession)
			protected.GET("/storage/:userId", handlers.GetStorageUsage)

			// MASTERY ROUTES (updated for nodes)
			protected.GET("/mastery/nodes/:nodeId", handlers.GetNodeMastery)
			protected.GET("/mastery/nodes", handlers.GetAllNodesMastery)
			protected.PUT("/mastery/nodes/:nodeId/refresh", handlers.RefreshNodeMastery)
			protected.GET("/mastery/stats", handlers.GetMasteryStats)
			// Legacy folder routes - deprecated but kept for compatibility
			protected.GET("/mastery/folders/:folderId", handlers.GetFolderMastery)
			protected.GET("/mastery/folders", handlers.GetAllFoldersMastery)
			protected.PUT("/mastery/notes/:noteId", handlers.UpdateNoteMastery)

			// INDEXING ROUTES
			protected.GET("/indexing/notes/:noteId", handlers.GetNoteIndexStatus)
			protected.GET("/indexing/folders/:folderId", handlers.GetFolderIndexStatus)
			protected.PUT("/indexing/notes/:noteId", handlers.UpdateDocumentIndex)
			protected.POST("/indexing/notes/:noteId/summary", handlers.GenerateSummary)

			// SESSION ROUTES
			protected.POST("/session/start", handlers.StartQuizSession)
			protected.PUT("/session/:sessionId/update", handlers.UpdateSessionProgress)
			protected.PUT("/session/:sessionId/complete", handlers.CompleteQuizSession)
			protected.GET("/session/active/:userId", handlers.GetActiveSession)

			// SEARCH ROUTES
			protected.GET("/search", handlers.SearchItems)

			// NODE ROUTES
			protected.POST("/nodes", handlers.CreateNode)
			protected.GET("/nodes/:id", handlers.GetNode)
			protected.GET("/nodes", handlers.GetNodeChildren)
			protected.PUT("/nodes/:id", handlers.UpdateNode)
			protected.DELETE("/nodes/:id", handlers.DeleteNode)
			protected.GET("/nodes/tree/:id", handlers.GetNodeTree)
			protected.POST("/nodes/notes", handlers.CreateNoteNode)
			protected.GET("/nodes/:id/image", handlers.GetNodeImage)
			protected.PUT("/nodes/:id/caption", handlers.RegenerateNodeCaption)
			protected.POST("/nodes/:id/review", handlers.ReviewNoteNode)
			protected.GET("/nodes/:id/name-suggestions", handlers.GetNameSuggestionsForFolder)

			// QUIZ ROUTES
			protected.POST("/quizzes/folders/:folderId", handlers.CreateQuiz)
			protected.POST("/quizzes/folders/:folderId/request", handlers.RequestQuizGeneration)
			protected.GET("/quizzes/folders/:folderId/status", handlers.GetQuizStatus)
			protected.GET("/quizzes/:quizId", handlers.GetQuiz)
			protected.GET("/quizzes/:quizId/questions", handlers.GetQuizQuestions)
			protected.POST("/quizzes/:quizId/questions/:questionId/answer", handlers.SubmitAnswer)
			protected.GET("/quizzes/:quizId/summary", handlers.GetQuizSummary)
			protected.POST("/quizzes/:quizId/regenerate", handlers.RegenerateQuiz)

			// REVIEW ROUTES
			protected.GET("/reviews/queue", handlers.GetReviewQueue)
			protected.GET("/reviews/note/:noteId/history", handlers.GetNoteReviewHistory)
			protected.PUT("/reviews/note/:noteId/status", handlers.UpdateReviewStatus)
		}
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Defer cleanup for background queues
	defer masteryQueue.Stop()

	log.Printf("Server starting on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
}

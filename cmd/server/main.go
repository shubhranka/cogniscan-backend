// ./cogniscan-backend/cmd/server/main.go
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strings"

	"cogniscan/backend/internal/database"
	"cogniscan/backend/internal/handlers"
	"cogniscan/backend/internal/middleware"
	"cogniscan/backend/internal/services"

	firebase "firebase.google.com/go/v4"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"google.golang.org/api/option"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, relying on environment variables")
	}
	database.ConnectDB()
	if err := services.InitMegaService(); err != nil {
		log.Fatalf("Failed to initialize MEGA Service: %v", err)
	}
	ctx := context.Background()
	keyDataString := os.Getenv("KEY_DATA")
	if keyDataString == "" {
		log.Fatal("KEY_DATA environment variable not set")
	}
	var keyData map[string]interface{}
	err := json.Unmarshal([]byte(keyDataString), &keyData)
	if err != nil {
		log.Fatalf("error unmarshalling key data: %v\n", err)
	}
	keyData["private_key"] = strings.ReplaceAll(keyData["private_key"].(string), "\\n", "\n")
	parsedKeyDataString, err := json.Marshal(keyData)
	if err != nil {
		log.Fatalf("error marshalling key data: %v\n", err)
	}
	opt := option.WithCredentialsJSON(parsedKeyDataString)
	app, err := firebase.NewApp(ctx, nil, opt)
	if err != nil {
		log.Fatalf("error initializing app: %v\n", err)
	}
	authClient, err := app.Auth(ctx)
	if err != nil {
		log.Fatalf("error getting Auth client: %v\n", err)
	}

	router := gin.Default()

	// Health Route
	router.GET("/health", handlers.HealthCheck)

	api := router.Group("/api/v1")
	{
		protected := api.Group("/").Use(middleware.AuthMiddleware(authClient))
		{
			// FOLDER ROUTES
			protected.POST("/folders", handlers.CreateFolder)
			protected.GET("/folders/:folderId", handlers.GetFolders)
			protected.PUT("/folders/:id", handlers.UpdateFolder)
			protected.DELETE("/folders/:id", handlers.DeleteFolder)

			// NOTE ROUTES
			protected.POST("/notes", handlers.CreateNote)
			protected.GET("/folders/:folderId/notes", handlers.GetNotesInFolder)
			protected.PUT("/notes/:id", handlers.UpdateNote)
			protected.DELETE("/notes/:id", handlers.DeleteNote)
			protected.GET("/notes/:id/image", handlers.GetNoteImage)

			// SEARCH ROUTE
			protected.GET("/search", handlers.SearchItems)
		}
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Server starting on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
}

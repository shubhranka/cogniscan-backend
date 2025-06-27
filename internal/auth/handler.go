package auth

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"cogniscan/backend/internal/database"
	"cogniscan/backend/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type LoginPayload struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func LoginHandler(c *gin.Context) {
	var payload LoginPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	// --- MEGA LOGIN SIMULATION ---
	// In a real application, you would use a Go MEGA client library here.
	// You'd pass payload.Email and payload.Password to the library's login function.
	// For now, we will simulate a successful login for the provided credentials.
	log.Printf("Simulating MEGA login for user: %s", payload.Email)
	if payload.Email != "test@example.com" || payload.Password != "password" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials (simulated)"})
		return
	}
	simulatedMegaSession := "simulated-mega-session-string-for-" + payload.Email
	// --- END SIMULATION ---

	// Find or create a user in our database
	usersCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("users")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user models.User
	filter := bson.M{"email": payload.Email}
	update := bson.M{
		"$set": bson.M{
			"email":       payload.Email,
			"megaSession": simulatedMegaSession, // Store the (simulated) session
		},
	}
	opts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)

	err := usersCollection.FindOneAndUpdate(ctx, filter, update, opts).Decode(&user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process user data"})
		return
	}

	// Generate JWT for our own API session management
	token, err := GenerateJWT(user.ID.Hex(), user.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": token})
}

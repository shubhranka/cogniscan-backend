package handlers

import (
	"context"
	"github.com/gin-gonic/gin"
	"firebase.google.com/go/v4/auth"
)

// setupTestRouterWithUserID creates a test router with a specific user ID
// This avoids conflicts with existing setupTestRouter in other test files
func setupTestRouterWithUserID(userID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Mock auth middleware that sets the specified user ID in both Gin and request context
	router.Use(func(c *gin.Context) {
		// Create a mock Firebase token
		token := &auth.Token{
			Claims: map[string]interface{}{
				"email": userID,
			},
		}

		// Set in Gin context (for handlers that use c.GetString("userId"))
		c.Set("userId", userID)

		// Set in request context (for handlers that use middleware.ForContext)
		type contextKey string
		ctx := context.WithValue(c.Request.Context(), contextKey("user"), token)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	})

	return router
}

// setupTestRouterNoAuth creates a test router without auth middleware
// for testing unauthorized access scenarios
func setupTestRouterNoAuth() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	return router
}

// assertJSONContentType checks if the content type is JSON
// This is a shared utility for test files
func assertJSONContentType(contentType string) bool {
	return contentType == "application/json; charset=utf-8" || contentType == "application/json"
}

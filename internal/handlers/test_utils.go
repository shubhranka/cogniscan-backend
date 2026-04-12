package handlers

import (
	"github.com/gin-gonic/gin"
)

// setupTestRouterWithUserID creates a test router with a specific user ID
// This avoids conflicts with existing setupTestRouter in other test files
func setupTestRouterWithUserID(userID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Mock auth middleware that sets the specified user ID in context
	router.Use(func(c *gin.Context) {
		c.Set("user", userID)
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

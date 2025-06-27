package middleware

import (
	"context"
	"log"
	"net/http"
	"strings"

	"firebase.google.com/go/v4/auth"
	"github.com/gin-gonic/gin"
)

// A private key for context access
type contextKey string

const userContextKey = contextKey("user")

// AuthMiddleware creates a middleware that verifies Firebase ID tokens.
func AuthMiddleware(client *auth.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.Request.Header.Get("Authorization")

		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization header is required"})
			return
		}

		tokenString := strings.Replace(authHeader, "Bearer ", "", 1)
		token, err := client.VerifyIDToken(context.Background(), tokenString)
		if err != nil {
			log.Printf("Error verifying Firebase ID token: %v", err)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid auth token"})
			return
		}

		// Store the verified token claims in the context for handlers to use
		ctx := context.WithValue(c.Request.Context(), userContextKey, token)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// ForContext finds the user from the context.
func ForContext(ctx context.Context) *auth.Token {
	raw, _ := ctx.Value(userContextKey).(*auth.Token)
	return raw
}

// ./cogniscan-backend/internal/handlers/chat_handler.go
package handlers

import (
	"net/http"

	"cogniscan/backend/internal/middleware"
	"cogniscan/backend/internal/services"
	"github.com/gin-gonic/gin"
)

// ChatPayload is the request body for sending a chat message
type ChatPayload struct {
	Message       string `json:"message" binding:"required"`
	FolderID      string `json:"folderId" binding:"required"`
	ConversationID string `json:"conversationId,omitempty"` // Optional for new chats
}

// CreateConversation creates a new chat conversation for a folder
func CreateConversation(c *gin.Context) {
	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var payload struct {
		FolderID string `json:"folderId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
		return
	}

	conversation, err := services.CreateConversation(
		c.Request.Context(),
		payload.FolderID,
		firebaseUser.UID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, conversation)
}

// SendMessage sends a user message and returns AI response with citations
func SendMessage(c *gin.Context) {
	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var payload ChatPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
		return
	}

	response, err := services.ProcessChatMessage(
		c.Request.Context(),
		payload.Message,
		payload.FolderID,
		firebaseUser.UID,
		payload.ConversationID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, response)
}

// GetConversations lists all conversations for a folder
func GetConversations(c *gin.Context) {
	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	folderID := c.Query("folderId")
	if folderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "folderId is required"})
		return
	}

	conversations, err := services.GetConversations(
		c.Request.Context(),
		folderID,
		firebaseUser.UID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"conversations": conversations})
}

// GetConversation retrieves a specific conversation with all messages
func GetConversation(c *gin.Context) {
	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	conversationID := c.Param("conversationId")

	conversation, err := services.GetConversation(
		c.Request.Context(),
		conversationID,
		firebaseUser.UID,
	)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Conversation not found"})
		return
	}

	c.JSON(http.StatusOK, conversation)
}

// DeleteConversation deletes a conversation
func DeleteConversation(c *gin.Context) {
	firebaseUser := middleware.ForContext(c.Request.Context())
	if firebaseUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	conversationID := c.Param("conversationId")

	err := services.DeleteConversation(
		c.Request.Context(),
		conversationID,
		firebaseUser.UID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Conversation deleted"})
}

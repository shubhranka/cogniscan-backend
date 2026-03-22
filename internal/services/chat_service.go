// ./cogniscan-backend/internal/services/chat_service.go
package services

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/shared"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"cogniscan/backend/internal/database"
	"cogniscan/backend/internal/models"
)

const (
	chatCollectionName = "chat_conversations"
	maxContextMessages = 10 // Include last 10 messages for context
	maxRetrievedNotes = 5  // Retrieve top 5 relevant notes
)

// getChatCollection returns the chat conversations collection
func getChatCollection() *mongo.Collection {
	return database.Client.Database(os.Getenv("DB_NAME")).Collection(chatCollectionName)
}

// ChatResponse represents the response to a chat message
type ChatResponse struct {
	Message        models.ChatMessage   `json:"message"`
	Citations      []models.ChatCitation `json:"citations"`
	ConversationID string             `json:"conversationId,omitempty"`
}

// CreateConversation creates a new chat conversation
func CreateConversation(ctx context.Context, folderID, ownerID string) (*models.ChatConversation, error) {
	collection := getChatCollection()

	now := time.Now()
	conversation := &models.ChatConversation{
		ID:        primitive.NewObjectID(),
		FolderID:   folderID,
		OwnerID:    ownerID,
		Title:      "New Conversation", // Will be updated on first message
		Messages:   []models.ChatMessage{},
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	result, err := collection.InsertOne(ctx, conversation)
	if err != nil {
		log.Printf("[ChatService] Failed to create conversation: %v", err)
		return nil, err
	}

	conversation.ID = result.InsertedID.(primitive.ObjectID)
	return conversation, nil
}

// ProcessChatMessage processes a user message and returns AI response with citations
func ProcessChatMessage(ctx context.Context, userMessage, folderID, ownerID, conversationID string) (*ChatResponse, error) {
	var conversation *models.ChatConversation
	var err error

	// Retrieve or create conversation
	if conversationID != "" {
		conversation, err = GetConversation(ctx, conversationID, ownerID)
		if err != nil {
			return nil, fmt.Errorf("conversation not found")
		}

		// Verify folder matches
		if conversation.FolderID != folderID {
			return nil, fmt.Errorf("conversation does not belong to this folder")
		}
	} else {
		// Create new conversation
		conv, err := CreateConversation(ctx, folderID, ownerID)
		if err != nil {
			return nil, err
		}
		conversation = conv
	}

	// Add user message to conversation
	userChatMsg := models.ChatMessage{
		ID:        primitive.NewObjectID(),
		Role:      "user",
		Content:   userMessage,
		CreatedAt: time.Now(),
	}

	conversation.Messages = append(conversation.Messages, userChatMsg)

	// Generate query embedding for semantic search
	queryEmbedding, err := GenerateQueryEmbedding(userMessage)
	if err != nil {
		log.Printf("[ChatService] Failed to generate query embedding: %v", err)
		// Continue without citations if embedding generation fails
	}

	// Retrieve relevant notes using vector search
	var relevantNotes []models.CaptionEmbedding
	var scores []float32

	if queryEmbedding != nil {
		relevantNotes, scores, err = SearchCaptionsInFolderWithEmbedding(
			queryEmbedding,
			maxRetrievedNotes,
			folderID,
			ownerID,
		)
		if err != nil {
			log.Printf("[ChatService] Failed to retrieve relevant notes: %v", err)
			// Continue without citations if search fails
		}
	}

	// Build context from retrieved notes
	contextText := buildContextFromNotes(relevantNotes, scores)

	// Build conversation history for context
	conversationHistory := buildConversationHistory(conversation.Messages)

	// Generate AI response
	aiResponse, err := generateChatResponse(userMessage, contextText, conversationHistory)
	if err != nil {
		return nil, fmt.Errorf("failed to generate AI response: %w", err)
	}

	// Add assistant message to conversation
	assistantChatMsg := models.ChatMessage{
		ID:        primitive.NewObjectID(),
		Role:      "assistant",
		Content:   aiResponse,
		CreatedAt: time.Now(),
	}
	conversation.Messages = append(conversation.Messages, assistantChatMsg)

	// Update conversation title if this is the first message
	if len(conversation.Messages) == 2 {
		conversation.Title = generateTitle(userMessage)
	}
	conversation.UpdatedAt = time.Now()

	// Save updated conversation
	if err := saveConversation(ctx, conversation); err != nil {
		log.Printf("[ChatService] Failed to save conversation: %v", err)
	}

	// Build citations
	citations := buildCitations(relevantNotes, scores)

	return &ChatResponse{
		Message:        assistantChatMsg,
		Citations:      citations,
		ConversationID: conversation.ID.Hex(),
	}, nil
}

// SearchCaptionsInFolderWithEmbedding performs vector search with pre-computed embedding
func SearchCaptionsInFolderWithEmbedding(
	embedding []float32,
	limit int,
	folderID, ownerID string,
) ([]models.CaptionEmbedding, []float32, error) {
	collection := getVectorCollection()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pipeline := mongo.Pipeline{
		bson.D{
			{Key: "$vectorSearch", Value: bson.M{
				"index":       vectorIndexName,
				"path":        "vector",
				"queryVector": embedding,
				"numCandidates": limit * 10,
				"limit":       limit,
				"filter": bson.M{
					"folderId": folderID,
					"ownerId":  ownerID,
				},
			}},
		},
		bson.D{
			{Key: "$project", Value: bson.M{
				"_id":       1,
				"noteId":    1,
				"folderId":  1,
				"ownerId":   1,
				"caption":   1,
				"score": bson.M{
					"$meta": "vectorSearchScore",
				},
			}},
		},
	}

	cursor, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, nil, err
	}
	defer cursor.Close(ctx)

	var results []models.CaptionEmbedding
	var scores []float32

	for cursor.Next(ctx) {
		var result struct {
			models.CaptionEmbedding
			Score float32 `bson:"score"`
		}
		if err := cursor.Decode(&result); err != nil {
			continue
		}
		results = append(results, result.CaptionEmbedding)
		scores = append(scores, result.Score)
	}

	return results, scores, nil
}

// buildContextFromNotes creates a formatted context string from retrieved notes
func buildContextFromNotes(notes []models.CaptionEmbedding, scores []float32) string {
	if len(notes) == 0 {
		return "No relevant notes found in this folder."
	}

	var builder strings.Builder
	builder.WriteString("RELEVANT NOTES FROM THE FOLDER:\n\n")

	for i, note := range notes {
		builder.WriteString(fmt.Sprintf("--- Note %d (Relevance: %.2f) ---\n", i+1, scores[i]))
		builder.WriteString(note.Caption)
		builder.WriteString("\n\n")
	}

	return builder.String()
}

// buildConversationHistory creates a formatted history string for the AI
func buildConversationHistory(messages []models.ChatMessage) string {
	if len(messages) == 0 {
		return ""
	}

	// Get last N messages for context
	start := 0
	if len(messages) > maxContextMessages {
		start = len(messages) - maxContextMessages
	}

	var builder strings.Builder
	builder.WriteString("PREVIOUS CONVERSATION:\n")

	for i := start; i < len(messages); i++ {
		msg := messages[i]
		builder.WriteString(fmt.Sprintf("%s: %s\n",
			strings.ToUpper(msg.Role),
			msg.Content))
	}

	return builder.String()
}

// generateChatResponse generates AI response using NVIDIA's LLaMA model
func generateChatResponse(userQuestion, contextText, conversationHistory string) (string, error) {
	systemPrompt := `You are a helpful AI assistant that answers questions based on the provided study materials. Your role is to help users understand and learn from their scanned notes.

IMPORTANT GUIDELINES:
1. Base your answers primarily on the provided context from the notes.
2. If the context doesn't contain enough information, acknowledge this limitation.
3. Cite the specific notes you used in your answer using [Note X] format.
4. Be concise but thorough in your explanations.
5. If asked to compare or summarize information, use the provided notes to do so.
6. Do not make up information that isn't in the notes.
7. If the question asks about something completely unrelated to the notes, politely explain that you can only help with the provided content.`

	var prompt string
	if conversationHistory != "" && !strings.HasPrefix(conversationHistory, "PREVIOUS CONVERSATION:\n") {
		prompt = fmt.Sprintf(`%s

%s

CURRENT USER QUESTION:
%s

Provide a helpful answer based on the notes above.`, systemPrompt, conversationHistory, userQuestion)
	} else {
		prompt = fmt.Sprintf(`%s

%s

CURRENT USER QUESTION:
%s

Provide a helpful answer based on the notes above.`, systemPrompt, contextText, userQuestion)
	}

	ctx := context.Background()
	completion, err := aiClient.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		},
		Model:       shared.ChatModel("meta/llama-3.3-70b-instruct"),
		MaxTokens:   openai.Int(2048),
		Temperature: openai.Float(0.70),
		TopP:        openai.Float(0.90),
	})

	if err != nil {
		return "", err
	}

	if len(completion.Choices) == 0 {
		return "", fmt.Errorf("no response from AI model")
	}

	return completion.Choices[0].Message.Content, nil
}

// buildCitations creates citation objects from retrieved notes
func buildCitations(notes []models.CaptionEmbedding, scores []float32) []models.ChatCitation {
	citations := make([]models.ChatCitation, 0, len(notes))

	for i, note := range notes {
		citations = append(citations, models.ChatCitation{
			NoteID:    note.NoteID,
			NoteName:  fmt.Sprintf("Note %d", i+1),
			Relevance: scores[i],
			Context:   note.Caption,
		})
	}

	return citations
}

// generateTitle creates a title for the conversation from the first message
func generateTitle(firstMessage string) string {
	// Truncate to reasonable length
	if len(firstMessage) > 50 {
		return firstMessage[:47] + "..."
	}
	return firstMessage
}

// saveConversation saves or updates a conversation
func saveConversation(ctx context.Context, conversation *models.ChatConversation) error {
	collection := getChatCollection()

	filter := bson.M{"_id": conversation.ID}
	update := bson.M{
		"$set": bson.M{
			"title":     conversation.Title,
			"messages":  conversation.Messages,
			"updatedAt": conversation.UpdatedAt,
		},
	}

	opts := options.Update().SetUpsert(true)
	_, err := collection.UpdateOne(ctx, filter, update, opts)
	return err
}

// GetConversation retrieves a conversation by ID
func GetConversation(ctx context.Context, conversationID, ownerID string) (*models.ChatConversation, error) {
	collection := getChatCollection()

	objID, err := primitive.ObjectIDFromHex(conversationID)
	if err != nil {
		return nil, err
	}

	var conversation models.ChatConversation
	err = collection.FindOne(ctx, bson.M{
		"_id":     objID,
		"ownerId": ownerID,
	}).Decode(&conversation)

	return &conversation, err
}

// GetConversations retrieves all conversations for a folder
func GetConversations(ctx context.Context, folderID, ownerID string) ([]models.ChatConversation, error) {
	collection := getChatCollection()

	cursor, err := collection.Find(ctx, bson.M{
		"folderId": folderID,
		"ownerId":  ownerID,
	})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var conversations []models.ChatConversation
	if err := cursor.All(ctx, &conversations); err != nil {
		return nil, err
	}

	return conversations, nil
}

// DeleteConversation deletes a conversation
func DeleteConversation(ctx context.Context, conversationID, ownerID string) error {
	collection := getChatCollection()

	objID, err := primitive.ObjectIDFromHex(conversationID)
	if err != nil {
		return err
	}

	_, err = collection.DeleteOne(ctx, bson.M{
		"_id":     objID,
		"ownerId": ownerID,
	})

	return err
}

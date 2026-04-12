package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
)

var aiClient openai.Client

// InitAIService initializes the OpenAI client with NVIDIA endpoint
func InitAIService() error {
	apiKey := os.Getenv("NVIDIA_API_KEY")
	if apiKey == "" {
		log.Println("[AIService] NVIDIA_API_KEY not set, caption generation disabled")
		return nil // Not fatal - caption generation will just fail gracefully
	}

	aiClient = openai.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL("https://integrate.api.nvidia.com/v1/"),
	)

	log.Println("[AIService] Successfully initialized AI client with NVIDIA endpoint")
	return nil
}

// isClientInitialized checks if the AI client has been initialized
func isClientInitialized() bool {
	return os.Getenv("NVIDIA_API_KEY") != ""
}

// GenerateCaption generates a full-text transcription of an image using NVIDIA's phi-3.5-vision-instruct
// Extracts handwritten and typed text, formulas, and diagram labels with maximum accuracy
func GenerateCaption(imageBytes []byte) (string, error) {
	if !isClientInitialized() {
		return "", fmt.Errorf("AI client not initialized")
	}

	// Encode image to base64
	base64Image := base64.StdEncoding.EncodeToString(imageBytes)

	// Prepare the content with image tag - using adaptive transcription prompt
	content := fmt.Sprintf(`<img src="data:image/jpeg;base64,%s">

Transcribe all visible text from this image. Describe any diagrams or tables with their labels. Copy mathematical formulas exactly. Do not use templates or placeholders - provide actual content only.`, base64Image)

	// Call the API
	ctx := context.Background()
	completion, err := aiClient.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(content),
		},
		Model:       shared.ChatModel("microsoft/phi-3.5-vision-instruct"),
		MaxTokens:   openai.Int(2048),   // Increased from 512 for comprehensive content extraction
		Temperature: openai.Float(0.30), // Balanced for flexibility across different content types
		TopP:        openai.Float(0.70),
	})

	if err != nil {
		log.Printf("[AIService] Failed to generate transcription: %v", err)
		return "", err
	}

	if len(completion.Choices) == 0 {
		return "", fmt.Errorf("no response from AI model")
	}

	result := completion.Choices[0].Message.Content
	if len(strings.TrimSpace(result)) == 0 {
		return "", fmt.Errorf("empty transcription returned - image may contain no text")
	}
	return result, nil
}

// GenerateEmbedding generates an embedding vector for the given text using NVIDIA's llama-nemotron-embed-1b-v2
// Uses "passage" input_type for storing captions in the database
func GenerateEmbedding(text string) ([]float32, error) {
	if !isClientInitialized() {
		return nil, fmt.Errorf("AI client not initialized")
	}

	ctx := context.Background()

	// Call the embeddings API with NVIDIA's embedding model
	// input_type is required for asymmetric models: "query" for search queries, "passage" for storing
	embedding, err := aiClient.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{
			OfArrayOfStrings: []string{text},
		},
		Model: openai.EmbeddingModel("nvidia/llama-nemotron-embed-1b-v2"),
	}, option.WithJSONSet("input_type", "passage"), option.WithJSONSet("truncate", "NONE"))

	if err != nil {
		log.Printf("[AIService] Failed to generate embedding: %v", err)
		return nil, err
	}

	if len(embedding.Data) == 0 {
		return nil, fmt.Errorf("no embedding data returned from AI model")
	}

	// Convert []float64 to []float32
	vec := embedding.Data[0].Embedding
	result := make([]float32, len(vec))
	for i, v := range vec {
		result[i] = float32(v)
	}

	return result, nil
}

// GenerateQueryEmbedding generates an embedding vector for a search query using NVIDIA's llama-nemotron-embed-1b-v2
// Uses "query" input_type for searching against stored document embeddings
func GenerateQueryEmbedding(text string) ([]float32, error) {
	if !isClientInitialized() {
		return nil, fmt.Errorf("AI client not initialized")
	}

	ctx := context.Background()

	// Call the embeddings API with NVIDIA's embedding model
	embedding, err := aiClient.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{
			OfArrayOfStrings: []string{text},
		},
		Model: openai.EmbeddingModel("nvidia/llama-nemotron-embed-1b-v2"),
	}, option.WithJSONSet("input_type", "query"), option.WithJSONSet("truncate", "NONE"))

	if err != nil {
		log.Printf("[AIService] Failed to generate query embedding: %v", err)
		return nil, err
	}

	if len(embedding.Data) == 0 {
		return nil, fmt.Errorf("no embedding data returned from AI model")
	}

	// Convert []float64 to []float32
	vec := embedding.Data[0].Embedding
	result := make([]float32, len(vec))
	for i, v := range vec {
		result[i] = float32(v)
	}

	return result, nil
}

// GenerateNameSuggestionsForNote generates name suggestions based on note caption
func GenerateNameSuggestionsForNote(caption string) ([]string, error) {
	if !isClientInitialized() {
		return nil, fmt.Errorf("AI client not initialized")
	}

	prompt := fmt.Sprintf(`You are helping a user name their scanned document note.

DOCUMENT DESCRIPTION:
%s

TASK: Generate 5-6 creative and descriptive name suggestions for this note.
Each suggestion should be:
1. Concise (2-6 words)
2. Descriptive of content
3. Professional yet friendly
4. Easy to remember

OUTPUT FORMAT (valid JSON array only, no markdown):
["Suggestion 1", "Suggestion 2", "Suggestion 3", "Suggestion 4", "Suggestion 5"]

Return only valid JSON, no surrounding text.`, caption)

	ctx := context.Background()
	completion, err := aiClient.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		},
		Model:       shared.ChatModel("meta/llama-3.3-70b-instruct"),
		MaxTokens:   openai.Int(512),
		Temperature: openai.Float(0.80),
		TopP:        openai.Float(0.90),
	})

	if err != nil {
		log.Printf("[AIService] Failed to generate name suggestions: %v", err)
		return nil, err
	}

	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("no response from AI model")
	}

	var suggestions []string
	if err := json.Unmarshal([]byte(completion.Choices[0].Message.Content), &suggestions); err != nil {
		log.Printf("[AIService] Failed to parse name suggestions: %v", err)
		return nil, err
	}

	return suggestions, nil
}

// GenerateNameSuggestionsForFolder generates name suggestions based on all note captions in folder
func GenerateNameSuggestionsForFolder(noteCaptions []string) ([]string, error) {
	if !isClientInitialized() {
		return nil, fmt.Errorf("AI client not initialized")
	}

	if len(noteCaptions) == 0 {
		return nil, fmt.Errorf("no notes in folder")
	}

	captionsText := ""
	for i, caption := range noteCaptions {
		captionsText += fmt.Sprintf("Note %d: %s\n\n", i+1, caption)
	}

	prompt := fmt.Sprintf(`You are helping a user name a folder containing scanned documents.

DOCUMENTS IN FOLDER:
%s

TASK: Generate 5-6 creative and descriptive name suggestions for this folder.
Consider common themes, topics, or subject matter across all documents.
Each suggestion should be:
1. Concise (2-6 words)
2. Representative of folder's content
3. Professional yet friendly
4. Easy to organize and remember

OUTPUT FORMAT (valid JSON array only, no markdown):
["Suggestion 1", "Suggestion 2", "Suggestion 3", "Suggestion 4", "Suggestion 5"]

Return only valid JSON, no surrounding text.`, captionsText)

	ctx := context.Background()
	completion, err := aiClient.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		},
		Model:       shared.ChatModel("meta/llama-3.3-70b-instruct"),
		MaxTokens:   openai.Int(512),
		Temperature: openai.Float(0.80),
		TopP:        openai.Float(0.90),
	})

	if err != nil {
		log.Printf("[AIService] Failed to generate folder name suggestions: %v", err)
		return nil, err
	}

	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("no response from AI model")
	}

	var suggestions []string
	if err := json.Unmarshal([]byte(completion.Choices[0].Message.Content), &suggestions); err != nil {
		log.Printf("[AIService] Failed to parse folder name suggestions: %v", err)
		return nil, err
	}

	return suggestions, nil
}

// ============ NEW FUNCTIONS FOR REDESIGNED FEATURES ============

// GenerateDocumentSummary generates an AI summary for a document note
func GenerateDocumentSummary(noteContent string, noteTitle string) (string, error) {
	if !isClientInitialized() {
		return "", fmt.Errorf("AI client not initialized")
	}

	prompt := fmt.Sprintf(`Generate a comprehensive summary of the following document:
TITLE: %s

CONTENT:
%s

TASK: Create a well-structured summary that:
1. Captures the main topics and key concepts
2. Highlights important definitions and formulas
3. Notes any examples or case studies
4. Is concise yet informative (100-200 words)
5. Uses bullet points for easy scanning
6. Ends with 3-5 key takeaways

OUTPUT FORMAT: Return only the summary text, no markdown or markdown formatting.`, noteTitle, noteContent)

	ctx := context.Background()
	completion, err := aiClient.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		},
		Model:       shared.ChatModel("gpt-4o-mini"),
		MaxTokens:   openai.Int(500),
		Temperature: openai.Float(0.70),
		TopP:        openai.Float(0.90),
	})

	if err != nil {
		return "", fmt.Errorf("failed to generate document summary: %w", err)
	}

	if len(completion.Choices) == 0 {
		return "", fmt.Errorf("no response from AI model")
	}

	return completion.Choices[0].Message.Content, nil
}

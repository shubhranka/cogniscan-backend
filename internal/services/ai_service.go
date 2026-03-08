package services

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"

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

// GenerateCaption generates a caption for an image using NVIDIA's phi-3.5-vision-instruct
func GenerateCaption(imageBytes []byte) (string, error) {
	if !isClientInitialized() {
		return "", fmt.Errorf("AI client not initialized")
	}

	// Encode image to base64
	base64Image := base64.StdEncoding.EncodeToString(imageBytes)

	// Prepare the content with image tag
	content := fmt.Sprintf("<img src=\"data:image/jpeg;base64,%s\">\n\nDescribe this image in a concise but descriptive way.", base64Image)

	// Call the API
	ctx := context.Background()
	completion, err := aiClient.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(content),
		},
		Model:       shared.ChatModel("microsoft/phi-3.5-vision-instruct"),
		MaxTokens:   openai.Int(512),
		Temperature: openai.Float(0.20),
		TopP:        openai.Float(0.70),
	})

	if err != nil {
		log.Printf("[AIService] Failed to generate caption: %v", err)
		return "", err
	}

	if len(completion.Choices) == 0 {
		return "", fmt.Errorf("no response from AI model")
	}

	return completion.Choices[0].Message.Content, nil
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

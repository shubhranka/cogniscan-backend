// main package for missing captions script
// This script truncates caption_embeddings and regenerates captions/embeddings for all note nodes
package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"encoding/json"

	"github.com/joho/godotenv"
	"github.com/openai/openai-go"
	oaioption "github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/oauth2"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

type NodeType string

const (
	NodeTypeFolder NodeType = "folder"
	NodeTypeNote   NodeType = "note"
)

type NodeMetadata struct {
	Type    NodeType `bson:"type"`
	DriveID string   `bson:"driveId,omitempty"`
}

type Node struct {
	ID        string       `bson:"_id"`
	Name      string       `bson:"name"`
	ParentID  string       `bson:"parentId"`
	Metadata  NodeMetadata `bson:"metadata"`
	OwnerID   string       `bson:"ownerId"`
	PublicURL string       `bson:"publicUrl,omitempty"`
}

type CaptionEmbedding struct {
	ID        string    `bson:"_id,omitempty"`
	NoteID    string    `bson:"noteId"`
	FolderID  string    `bson:"folderId"`
	OwnerID   string    `bson:"ownerId"`
	Caption   string    `bson:"caption"`
	Vector    []float32 `bson:"vector"`
	CreatedAt time.Time `bson:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt"`
}

var (
	mongoClient *mongo.Client
	aiClient    openai.Client
	driveSrv    *drive.Service
)

func init() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: Error loading .env file")
	}
}

func main() {
	log.Println("=== Missing Captions Script ===")
	log.Println("This script will truncate caption_embeddings and regenerate captions for all note nodes.")

	// Initialize services
	if err := initServices(); err != nil {
		log.Fatalf("Failed to initialize services: %v", err)
	}
	log.Println("All services initialized successfully")

	// Truncate caption_embeddings collection
	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "cogniscan"
	}

	log.Println("\n=== Truncating caption_embeddings collection ===")
	embeddingsCollection := mongoClient.Database(dbName).Collection("caption_embeddings")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	count, err := embeddingsCollection.CountDocuments(ctx, bson.M{})
	if err != nil {
		log.Fatalf("Failed to count caption_embeddings: %v", err)
	}

	if count > 0 {
		fmt.Printf("Found %d existing caption embeddings. Delete and regenerate? (y/n): ", count)
		var confirmation string
		fmt.Scanln(&confirmation)
		if confirmation != "y" && confirmation != "Y" {
			log.Println("Aborted by user")
			return
		}

		if err := embeddingsCollection.Drop(ctx); err != nil {
			log.Fatalf("Failed to drop caption_embeddings collection: %v", err)
		}
		log.Printf("Dropped caption_embeddings collection (deleted %d documents)", count)
	} else {
		log.Println("caption_embeddings collection is empty")
	}

	// Find all note nodes
	log.Println("\n=== Finding all note nodes ===")
	nodes, err := findAllNoteNodes(ctx, dbName)
	if err != nil {
		log.Fatalf("Failed to find note nodes: %v", err)
	}

	if len(nodes) == 0 {
		log.Println("No note nodes found!")
		return
	}

	log.Printf("Found %d note nodes\n", len(nodes))

	// Ask for confirmation
	fmt.Printf("Process %d note nodes? (y/n): ", len(nodes))
	var confirmation string
	fmt.Scanln(&confirmation)
	if confirmation != "y" && confirmation != "Y" {
		log.Println("Aborted by user")
		return
	}

	// Process each note node
	successCount := 0
	failureCount := 0

	for i, node := range nodes {
		log.Printf("\n[%d/%d] Processing note node: %s (ID: %s)", i+1, len(nodes), node.Name, node.ID)

		if err := processNoteNode(node, dbName); err != nil {
			log.Printf("  ERROR: Failed to process note node: %v", err)
			failureCount++
		} else {
			log.Printf("  SUCCESS: Caption and embedding generated")
			successCount++
		}

		// Small delay between requests to avoid rate limiting
		if i < len(nodes)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	log.Printf("\n=== Summary ===")
	log.Printf("Total note nodes: %d", len(nodes))
	log.Printf("Successful: %d", successCount)
	log.Printf("Failed: %d", failureCount)
}

func initServices() error {
	if err := initMongoDB(); err != nil {
		return fmt.Errorf("MongoDB: %w", err)
	}
	if err := initAIService(); err != nil {
		return fmt.Errorf("AI Service: %w", err)
	}
	if err := initDriveService(); err != nil {
		return fmt.Errorf("Drive Service: %w", err)
	}
	return nil
}

func initMongoDB() error {
	log.Println("Initializing MongoDB...")
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		return fmt.Errorf("MONGO_URI environment variable not set")
	}

	client, err := mongo.NewClient(options.Client().ApplyURI(mongoURI))
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		return err
	}

	if err := client.Ping(ctx, nil); err != nil {
		return err
	}

	mongoClient = client
	log.Println("  MongoDB connected")
	return nil
}

func initAIService() error {
	log.Println("Initializing AI Service...")
	apiKey := os.Getenv("NVIDIA_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("NVIDIA_API_KEY environment variable not set")
	}

	aiClient = openai.NewClient(
		oaioption.WithAPIKey(apiKey),
		oaioption.WithBaseURL("https://integrate.api.nvidia.com/v1/"),
	)

	log.Println("  AI Service initialized")
	return nil
}

func initDriveService() error {
	log.Println("Initializing Drive Service...")
	credentialsJSONString := os.Getenv("COGNI_BACKEND")
	if credentialsJSONString == "" {
		return fmt.Errorf("COGNI_BACKEND environment variable not set")
	}

	srv, err := drive.NewService(context.Background(), option.WithCredentialsFile("service-account.json"))
	if err != nil {
		return fmt.Errorf("unable to retrieve Drive client: %v", err)
	}

	driveSrv = srv
	log.Println("  Drive Service initialized")
	return nil
}

func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

func saveToken(file string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", file)
	f, err := os.Create(file)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func findAllNoteNodes(ctx context.Context, dbName string) ([]Node, error) {
	nodesCollection := mongoClient.Database(dbName).Collection("nodes")

	filter := bson.M{
		"metadata.type": NodeTypeNote,
	}

	cursor, err := nodesCollection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var nodes []Node
	if err := cursor.All(ctx, &nodes); err != nil {
		return nil, err
	}

	return nodes, nil
}

func processNoteNode(node Node, dbName string) error {
	if node.Metadata.DriveID == "" {
		return fmt.Errorf("note node has no Drive ID")
	}

	// 1. Download image from Drive
	resp, err := driveSrv.Files.Get(node.Metadata.DriveID).Download()
	if err != nil {
		return fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	imageBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read image bytes: %w", err)
	}

	// 2. Generate caption
	caption, err := generateCaption(imageBytes)
	if err != nil {
		return fmt.Errorf("failed to generate caption: %w", err)
	}

	log.Printf("  Generated caption: %s", caption)

	// 3. Generate embedding
	vector, err := generateEmbedding(caption)
	if err != nil {
		return fmt.Errorf("failed to generate embedding: %w", err)
	}

	log.Printf("  Generated embedding (dim: %d)", len(vector))

	// 4. Store in caption_embeddings collection
	embeddingsCollection := mongoClient.Database(dbName).Collection("caption_embeddings")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	embedding := CaptionEmbedding{
		NoteID:    node.ID,
		FolderID:  node.ParentID,
		OwnerID:   node.OwnerID,
		Caption:   caption,
		Vector:    vector,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if _, err := embeddingsCollection.InsertOne(ctx, embedding); err != nil {
		return fmt.Errorf("failed to store embedding: %w", err)
	}

	return nil
}

func generateCaption(imageBytes []byte) (string, error) {
	base64Image := base64.StdEncoding.EncodeToString(imageBytes)
	content := fmt.Sprintf(`<img src="data:image/jpeg;base64,%s">

Transcribe all visible text from this image. Describe any diagrams or tables with their labels. Copy mathematical formulas exactly. Do not use templates or placeholders - provide actual content only.`, base64Image)

	ctx := context.Background()
	completion, err := aiClient.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(content),
		},
		// Model:       shared.ChatModel("microsoft/phi-3.5-vision-instruct"),
		Model:       shared.ChatModel("microsoft/phi-4-multimodal-instruct"),
		MaxTokens:   openai.Int(2048),
		Temperature: openai.Float(0.30),
		TopP:        openai.Float(0.70),
	})

	if err != nil {
		return "", err
	}

	if len(completion.Choices) == 0 {
		return "", fmt.Errorf("no response from AI model")
	}

	result := completion.Choices[0].Message.Content
	return result, nil
}

func generateEmbedding(text string) ([]float32, error) {
	ctx := context.Background()

	embeddingsResponse, err := aiClient.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{
			OfArrayOfStrings: []string{text},
		},
		Model: openai.EmbeddingModel("nvidia/llama-nemotron-embed-1b-v2"),
	}, oaioption.WithJSONSet("input_type", "passage"), oaioption.WithJSONSet("truncate", "NONE"))

	if err != nil {
		return nil, err
	}

	if len(embeddingsResponse.Data) == 0 {
		return nil, fmt.Errorf("no embedding data returned from AI model")
	}

	vec := embeddingsResponse.Data[0].Embedding
	result := make([]float32, len(vec))
	for i, v := range vec {
		result[i] = float32(v)
	}

	return result, nil
}

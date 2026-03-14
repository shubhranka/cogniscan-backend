// main package for the missing captions script
// This script finds notes without captions and generates captions and embeddings for them
package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/openai/openai-go"
	oaioption "github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"encoding/json"
)

type Note struct {
	ID            string `bson:"_id"`
	Name          string `bson:"name"`
	PublicURL     string `bson:"publicUrl"`
	DriveID       string `bson:"driveId"`
	Caption       string `bson:"caption"`
	CaptionStatus string `bson:"captionStatus"`
	CaptionError  string `bson:"captionError"`
	FolderID      string `bson:"folderId"`
	OwnerID       string `bson:"ownerId"`
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

func main() {
	log.Println("=== Missing Captions Script ===")
	log.Println("This script will find notes without captions and generate captions + embeddings for them.")

	// Initialize services
	if err := initServices(); err != nil {
		log.Fatalf("Failed to initialize services: %v", err)
	}
	log.Println("All services initialized successfully")

	// Find notes without captions
	notes, err := findNotesWithoutCaptions()
	if err != nil {
		log.Fatalf("Failed to find notes: %v", err)
	}

	if len(notes) == 0 {
		log.Println("No notes found without captions. All notes have captions!")
		return
	}

	log.Printf("Found %d notes without captions\n", len(notes))

	// Ask for confirmation
	fmt.Printf("\nProcess %d notes? (y/n): ", len(notes))
	var confirmation string
	fmt.Scanln(&confirmation)
	if confirmation != "y" && confirmation != "Y" {
		log.Println("Aborted by user")
		return
	}

	// Process each note
	successCount := 0
	failureCount := 0

	for i, note := range notes {
		log.Printf("\n[%d/%d] Processing note: %s (ID: %s)", i+1, len(notes), note.Name, note.ID)

		if err := processNote(note); err != nil {
			log.Printf("  ERROR: Failed to process note: %v", err)
			failureCount++
		} else {
			log.Printf("  SUCCESS: Caption and embedding generated")
			successCount++
		}

		// Small delay between requests to avoid rate limiting
		if i < len(notes)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	log.Printf("\n=== Summary ===")
	log.Printf("Total notes: %d", len(notes))
	log.Printf("Successful: %d", successCount)
	log.Printf("Failed: %d", failureCount)
}

func initServices() error {
	// Initialize MongoDB
	if err := initMongoDB(); err != nil {
		return fmt.Errorf("MongoDB: %w", err)
	}

	// Initialize AI Service
	if err := initAIService(); err != nil {
		return fmt.Errorf("AI Service: %w", err)
	}

	// Initialize Drive Service
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
	ctx := context.Background()

	credentialsJSONString := os.Getenv("COGNI_BACKEND")
	if credentialsJSONString == "" {
		return fmt.Errorf("COGNI_BACKEND environment variable not set")
	}

	config, err := google.ConfigFromJSON([]byte(credentialsJSONString), drive.DriveFileScope)
	if err != nil {
		return fmt.Errorf("unable to parse client secret to config: %v", err)
	}

	// Try to read token from file
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		return fmt.Errorf("token file not found. Please run the main server first to generate token.json: %v", err)
	}

	client := config.Client(context.Background(), tok)

	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
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

func findNotesWithoutCaptions() ([]Note, error) {
	log.Println("Finding notes without captions...")

	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "cogniscan"
	}

	notesCollection := mongoClient.Database(dbName).Collection("notes")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Find notes where caption is empty OR captionStatus is not "completed"
	filter := bson.M{
		"$or": []bson.M{
			{"caption": bson.M{"$exists": false}},
			{"caption": ""},
			{"captionStatus": bson.M{"$ne": "completed"}},
		},
	}

	cursor, err := notesCollection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var notes []Note
	if err := cursor.All(ctx, &notes); err != nil {
		return nil, err
	}

	log.Printf("  Found %d notes without captions", len(notes))
	return notes, nil
}

func processNote(note Note) error {
	// 1. Download image from Drive
	resp, err := driveSrv.Files.Get(note.DriveID).Download()
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

	// 4. Update note with caption and status
	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "cogniscan"
	}

	notesCollection := mongoClient.Database(dbName).Collection("notes")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	noteObjID, err := primitiveObjectIDFromHex(note.ID)
	if err != nil {
		return fmt.Errorf("invalid note ID: %w", err)
	}

	filter := bson.M{"_id": noteObjID}
	update := bson.M{
		"$set": bson.M{
			"caption":       caption,
			"captionStatus": "completed",
			"updatedAt":     time.Now(),
		},
		"$unset": bson.M{
			"captionError": "",
		},
	}

	if _, err := notesCollection.UpdateOne(ctx, filter, update); err != nil {
		return fmt.Errorf("failed to update note: %w", err)
	}

	// 5. Store embedding in caption_embeddings collection
	embeddingsCollection := mongoClient.Database(dbName).Collection("caption_embeddings")
	embeddingFilter := bson.M{"noteId": note.ID}
	embeddingUpdate := bson.M{
		"$set": bson.M{
			"noteId":    note.ID,
			"folderId":  note.FolderID,
			"ownerId":   note.OwnerID,
			"caption":   caption,
			"vector":    vector,
			"updatedAt": time.Now(),
		},
		"$setOnInsert": bson.M{
			"createdAt": time.Now(),
		},
	}

	opts := options.Update().SetUpsert(true)
	if _, err := embeddingsCollection.UpdateOne(ctx, embeddingFilter, embeddingUpdate, opts); err != nil {
		return fmt.Errorf("failed to store embedding: %w", err)
	}

	return nil
}

func generateCaption(imageBytes []byte) (string, error) {
	base64Image := base64.StdEncoding.EncodeToString(imageBytes)
	content := fmt.Sprintf("<img src=\"data:image/jpeg;base64,%s\">\n\nDescribe this image in a concise but descriptive way.", base64Image)

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
		return "", err
	}

	if len(completion.Choices) == 0 {
		return "", fmt.Errorf("no response from AI model")
	}

	return completion.Choices[0].Message.Content, nil
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

func primitiveObjectIDFromHex(s string) (primitive.ObjectID, error) {
	return primitive.ObjectIDFromHex(s)
}

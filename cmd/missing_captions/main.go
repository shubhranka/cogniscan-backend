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
	"strings"
	"time"

	"encoding/json"

	"github.com/joho/godotenv"
	"github.com/openai/openai-go"
	oaioption "github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
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

// Use dotenv to load environment variables
func init() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: Error loading .env file")
	}
}

func main() {
	log.Println("=== Missing Transcriptions Script ===")
	log.Println("This script will find notes without transcriptions and generate transcriptions + embeddings for them.")

	// Initialize services
	if err := initServices(); err != nil {
		log.Fatalf("Failed to initialize services: %v", err)
	}
	log.Println("All services initialized successfully")

	// query := "('me' in owners or 'me' in writers) and trashed = false"

	// // list all the files/folders of the drive
	// files, err := driveSrv.Files.List().IncludeItemsFromAllDrives(true).SupportsAllDrives(true).Do()
	// if err != nil {
	// 	log.Fatalf("Failed to list files: %v", err)
	// }
	// log.Printf("Found %d files", len(files.Files))
	// for _, file := range files.Files {
	// 	log.Printf("  - %s (ID: %s)", file.Name, file.Id)
	// 	if file.OwnedByMe {
	// 		log.Printf("    Owned by me: true")
	// 	} else {
	// 		log.Printf("    Owned by me: false")
	// 	}
	// 	if file.Capabilities != nil {
	// 		log.Printf("    Can Edit: %v", file.Capabilities.CanEdit)
	// 	} else {
	// 		log.Printf("    Can Edit: false (no capabilities)")
	// 	}
	// }

	// return

	// Find notes without transcriptions
	notes, err := findNotesWithoutTranscriptions()
	if err != nil {
		log.Fatalf("Failed to find notes: %v", err)
	}

	if len(notes) == 0 {
		log.Println("No notes found without transcriptions. All notes have transcriptions!")
		return
	}

	log.Printf("Found %d notes without transcriptions\n", len(notes))

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
			log.Printf("  SUCCESS: Transcription and embedding generated")
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
		// Generate new token
		log.Println("Generating new token...")

		// drive.DriveMetadataReadonlyScope
		// drive.DriveReadonlyScope
		config.Scopes = []string{drive.DriveMetadataReadonlyScope, drive.DriveReadonlyScope, drive.DriveFileScope}

		authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
		fmt.Println("Please visit the following URL to authorize this application:")
		fmt.Println(authURL)
		fmt.Println("Required scopes: " + strings.Join(config.Scopes, " "))
		fmt.Println("After authorization, please enter the authorization code:")
		var authCode string
		if _, err := fmt.Scan(&authCode); err != nil {
			return fmt.Errorf("unable to read authorization code: %v", err)
		}
		tok, err = config.Exchange(context.Background(), authCode)
		if err != nil {
			return fmt.Errorf("unable to retrieve token from web: %v", err)
		}

		saveToken(tokFile, tok)
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

func saveToken(file string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", file)
	f, err := os.Create(file)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

// findNotesWithoutTranscriptions finds notes that need transcriptions
// Criteria: caption is empty OR captionStatus is not "completed"
func findNotesWithoutTranscriptions() ([]Note, error) {
	log.Println("Finding notes without transcriptions...")

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

	log.Printf("  Found %d notes without transcriptions", len(notes))
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

	// 2. Generate transcription
	transcription, err := generateTranscription(imageBytes)
	if err != nil {
		return fmt.Errorf("failed to generate transcription: %w", err)
	}

	log.Printf("  Generated transcription: %s", transcription)

	// 3. Generate embedding
	vector, err := generateEmbedding(transcription)
	if err != nil {
		return fmt.Errorf("failed to generate embedding: %w", err)
	}

	log.Printf("  Generated embedding (dim: %d)", len(vector))

	// 4. Update note with transcription and status
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
			"caption":       transcription,
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

	// 5. Store embedding in caption_embeddings collection (field names unchanged for compatibility)
	embeddingsCollection := mongoClient.Database(dbName).Collection("caption_embeddings")
	embeddingFilter := bson.M{"noteId": note.ID}
	embeddingUpdate := bson.M{
		"$set": bson.M{
			"noteId":    note.ID,
			"folderId":  note.FolderID,
			"ownerId":   note.OwnerID,
			"caption":   transcription,
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

func generateTranscription(imageBytes []byte) (string, error) {
	base64Image := base64.StdEncoding.EncodeToString(imageBytes)
	// Using adaptive transcription prompt for comprehensive content extraction
	content := fmt.Sprintf(`<img src="data:image/jpeg;base64,%s">

Transcribe all visible text from this image. Describe any diagrams or tables with their labels. Copy mathematical formulas exactly. Do not use templates or placeholders - provide actual content only.`, base64Image)

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

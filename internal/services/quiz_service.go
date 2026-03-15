package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/shared"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"cogniscan/backend/internal/database"
	"cogniscan/backend/internal/models"
)

var quizModel = shared.ChatModel("meta/llama-3.3-70b-instruct")

// GetNotesForFolder retrieves all notes in a folder and its subfolders
func GetNotesForFolder(ctx context.Context, folderID, ownerID string) ([]models.Note, error) {
	collection := database.Client.Database("cogniscan").Collection("notes")

	// Get all folder IDs including nested folders
	folderIDs := []string{folderID}
	folderIDs = append(folderIDs, getNestedFolderIDs(ctx, folderID, ownerID)...)

	filter := bson.M{
		"folderId": bson.M{"$in": folderIDs},
		"ownerId":  ownerID,
		"caption":  bson.M{"$ne": ""}, // Only notes with captions
	}

	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}

	var notes []models.Note
	if err := cursor.All(ctx, &notes); err != nil {
		return nil, err
	}

	return notes, nil
}

func getNestedFolderIDs(ctx context.Context, parentID, ownerID string) []string {
	collection := database.Client.Database("cogniscan").Collection("folders")
	var folders []models.Folder

	filter := bson.M{"parentId": parentID, "ownerId": ownerID}
	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		return nil
	}

	if err := cursor.All(ctx, &folders); err != nil {
		return nil
	}

	ids := make([]string, 0, len(folders))
	for _, folder := range folders {
		ids = append(ids, folder.ID.Hex())
		// Recursively get nested folders
		nested := getNestedFolderIDs(ctx, folder.ID.Hex(), ownerID)
		ids = append(ids, nested...)
	}

	return ids
}

// UpdateFolderQuizStatus updates the quiz generation status for a folder
func UpdateFolderQuizStatus(ctx context.Context, folderID, ownerID string, status models.QuizGenerationStatus, quizID string, errorMsg string) error {
	collection := database.Client.Database("cogniscan").Collection("folders")

	objID, err := primitive.ObjectIDFromHex(folderID)
	if err != nil {
		return err
	}

	update := bson.M{
		"$set": bson.M{
			"quizGenerationStatus": status,
			"quizUpdatedAt":        time.Now(),
		},
	}

	if quizID != "" {
		update["$set"].(bson.M)["quizId"] = quizID
	} else {
		update["$unset"] = bson.M{"quizId": ""}
	}

	if errorMsg != "" {
		update["$set"].(bson.M)["quizError"] = errorMsg
	} else if update["$unset"] == nil {
		update["$unset"] = bson.M{"quizError": ""}
	} else {
		update["$unset"].(bson.M)["quizError"] = ""
	}

	filter := bson.M{"_id": objID, "ownerId": ownerID}
	_, err = collection.UpdateOne(ctx, filter, update)
	return err
}

// FolderQuizStatus represents the quiz generation status response
type FolderQuizStatus struct {
	Status   models.QuizGenerationStatus `json:"status"`
	QuizID   string                      `json:"quizId"`
	ErrorMsg string                      `json:"errorMsg"`
}

// GetFolderQuizStatus retrieves the quiz generation status for a folder
func GetFolderQuizStatus(ctx context.Context, folderID, ownerID string) (*FolderQuizStatus, error) {
	collection := database.Client.Database("cogniscan").Collection("folders")

	objID, err := primitive.ObjectIDFromHex(folderID)
	if err != nil {
		return nil, err
	}

	var folder struct {
		QuizGenerationStatus models.QuizGenerationStatus `bson:"quizGenerationStatus"`
		QuizID               string                      `bson:"quizId"`
		QuizError            string                      `bson:"quizError"`
	}

	err = collection.FindOne(ctx, bson.M{"_id": objID, "ownerId": ownerID}).Decode(&folder)
	if err != nil {
		return nil, err
	}

	// Default to "none" if status is empty
	status := folder.QuizGenerationStatus
	if status == "" {
		status = models.QuizGenStatusNone
	}

	return &FolderQuizStatus{
		Status:   status,
		QuizID:   folder.QuizID,
		ErrorMsg: folder.QuizError,
	}, nil
}

// GenerateQuestionsUsingAI generates questions using NVIDIA's LLaMA model
func GenerateQuestionsUsingAI(ctx context.Context, notes []models.Note) ([]models.Question, error) {
	if len(notes) == 0 {
		return nil, fmt.Errorf("no notes provided for question generation")
	}

	// Build note context
	noteContext := ""
	for _, note := range notes {
		noteContext += fmt.Sprintf("Note ID: %s\nCaption: %s\n\n", note.ID.Hex(), note.Caption)
	}

	prompt := fmt.Sprintf(`You are an educational content creator for a learning app. Generate multiple-choice quiz questions from the provided study materials.

STUDY MATERIAL (Full Transcriptions):
%s

GENERATION REQUIREMENTS:
1. Analyze the provided transcriptions which contain complete text from study materials
2. Determine how many questions would be appropriate based on:
   - The amount and complexity of content in the notes
   - Ensuring comprehensive coverage of the material
   - Avoiding redundancy in questions
3. Generate between 3 and 15 questions (adjust based on content volume)
4. Each question should reference between 2-4 different notes from the list above
5. Questions should be moderate difficulty - challenging but fair
6. Include a brief explanation for the correct answer
7. Each question should test understanding, not just recall

QUESTION TYPES TO INCLUDE (when applicable):
- Factual questions about specific terms, dates, or data points
- Conceptual questions testing understanding of principles
- Comparative questions asking about relationships between concepts
- Application questions requiring use of formulas or methods

OUTPUT FORMAT (valid JSON array only, no markdown):
[
  {
    "text": "Question text here",
    "options": ["Option A", "Option B", "Option C", "Option D"],
    "correctOption": 0,
    "referencedNoteIds": ["noteId1", "noteId2"],
    "explanation": "Brief explanation of why this is correct"
  }
]

IMPORTANT:
- correctOption is a zero-based index (0-3)
- referencedNoteIds must contain the exact note IDs from the input
- Questions should test specific facts and understanding from the transcriptions
- Return only valid JSON, no surrounding text`, noteContext)

	// Call NVIDIA API
	completion, err := aiClient.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		},
		Model:       quizModel,
		MaxTokens:   openai.Int(6144), // Increased for potentially more questions
		Temperature: openai.Float(0.70),
		TopP:        openai.Float(0.90),
	})

	if err != nil {
		return nil, fmt.Errorf("AI API error: %w", err)
	}

	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("no response from AI model")
	}

	// Parse JSON response
	var questions []models.Question
	if err := json.Unmarshal([]byte(completion.Choices[0].Message.Content), &questions); err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %w", err)
	}

	// Ensure at least some questions were generated
	if len(questions) == 0 {
		return nil, fmt.Errorf("AI generated no questions")
	}

	// Cap at reasonable maximum
	if len(questions) > 30 {
		questions = questions[:30]
	}

	return questions, nil
}

// CreateQuizForFolder creates a quiz for a folder
// If updateStatus is true, updates folder's quiz generation status throughout the process
func CreateQuizForFolder(ctx context.Context, folderID, ownerID string, updateStatus bool) (*models.Quiz, []models.Question, error) {
	// Mark as processing when worker starts (for async mode)
	if updateStatus {
		if err := UpdateFolderQuizStatus(ctx, folderID, ownerID, models.QuizGenStatusProcessing, "", ""); err != nil {
			return nil, nil, err
		}
	}

	// Get notes for folder
	notes, err := GetNotesForFolder(ctx, folderID, ownerID)
	if err != nil {
		if updateStatus {
			UpdateFolderQuizStatus(ctx, folderID, ownerID, models.QuizGenStatusFailed, "", fmt.Sprintf("failed to get notes: %v", err))
		}
		return nil, nil, fmt.Errorf("failed to get notes: %w", err)
	}

	if len(notes) == 0 {
		if updateStatus {
			UpdateFolderQuizStatus(ctx, folderID, ownerID, models.QuizGenStatusFailed, "", "no notes found in folder")
		}
		return nil, nil, fmt.Errorf("no notes found in folder")
	}

	// Generate questions
	questions, err := GenerateQuestionsUsingAI(ctx, notes)
	if err != nil {
		if updateStatus {
			UpdateFolderQuizStatus(ctx, folderID, ownerID, models.QuizGenStatusFailed, "", fmt.Sprintf("failed to generate questions: %v", err))
		}
		return nil, nil, fmt.Errorf("failed to generate questions: %w", err)
	}

	// Create quiz
	nowTime := time.Now()
	quiz := &models.Quiz{
		FolderID:       folderID,
		OwnerID:        ownerID,
		Status:         models.QuizStatusCompleted,
		TotalQuestions: len(questions),
		CorrectAnswers: 0, // Initialize to 0
		CreatedAt:      nowTime,
		UpdatedAt:      nowTime,
	}

	quizzesCollection := database.Client.Database("cogniscan").Collection("quizzes")
	result, err := quizzesCollection.InsertOne(ctx, quiz)
	if err != nil {
		if updateStatus {
			UpdateFolderQuizStatus(ctx, folderID, ownerID, models.QuizGenStatusFailed, "", fmt.Sprintf("failed to create quiz: %v", err))
		}
		return nil, nil, fmt.Errorf("failed to create quiz: %w", err)
	}

	quiz.ID = result.InsertedID.(primitive.ObjectID)

	// Set QuizID for questions and insert
	for i := range questions {
		questions[i].QuizID = quiz.ID.Hex()
		questions[i].CreatedAt = time.Now()
	}

	questionsCollection := database.Client.Database("cogniscan").Collection("questions")
	if _, err := questionsCollection.InsertMany(ctx, convertQuestionsToInterface(questions)); err != nil {
		if updateStatus {
			UpdateFolderQuizStatus(ctx, folderID, ownerID, models.QuizGenStatusFailed, "", fmt.Sprintf("failed to save questions: %v", err))
		}
		return nil, nil, fmt.Errorf("failed to save questions: %w", err)
	}

	// Initialize NoteReview entries for all notes
	for _, note := range notes {
		InitializeNoteReview(ctx, note.ID.Hex(), ownerID)
	}

	// Update folder status to completed
	if updateStatus {
		if err := UpdateFolderQuizStatus(ctx, folderID, ownerID, models.QuizGenStatusCompleted, quiz.ID.Hex(), ""); err != nil {
			// Log but don't fail the entire operation
		}
	}

	return quiz, questions, nil
}

func convertQuestionsToInterface(questions []models.Question) []interface{} {
	result := make([]interface{}, len(questions))
	for i, q := range questions {
		result[i] = q
	}
	return result
}

// GetQuiz retrieves a quiz
func GetQuiz(ctx context.Context, quizID, ownerID string) (*models.Quiz, error) {
	collection := database.Client.Database("cogniscan").Collection("quizzes")
	objID, err := primitive.ObjectIDFromHex(quizID)
	if err != nil {
		return nil, err
	}

	var quiz models.Quiz
	err = collection.FindOne(ctx, bson.M{"_id": objID, "ownerId": ownerID}).Decode(&quiz)
	return &quiz, err
}

// GetQuizQuestions retrieves all questions for a quiz
func GetQuizQuestions(ctx context.Context, quizID string) ([]models.Question, error) {
	collection := database.Client.Database("cogniscan").Collection("questions")

	cursor, err := collection.Find(ctx, bson.M{"quizId": quizID})
	if err != nil {
		return nil, err
	}

	var questions []models.Question
	if err := cursor.All(ctx, &questions); err != nil {
		return nil, err
	}

	return questions, nil
}

// GetQuestion retrieves a single question
func GetQuestion(ctx context.Context, questionID string) (*models.Question, error) {
	collection := database.Client.Database("cogniscan").Collection("questions")
	objID, err := primitive.ObjectIDFromHex(questionID)
	if err != nil {
		return nil, err
	}

	var question models.Question
	err = collection.FindOne(ctx, bson.M{"_id": objID}).Decode(&question)
	return &question, err
}

// GetQuizCollection returns the quizzes collection
func GetQuizCollection() *mongo.Collection {
	return database.Client.Database("cogniscan").Collection("quizzes")
}

// GetQuestionCollection returns the questions collection
func GetQuestionCollection() *mongo.Collection {
	return database.Client.Database("cogniscan").Collection("questions")
}

// GetAnswerCollection returns the question_answers collection
func GetAnswerCollection() *mongo.Collection {
	return database.Client.Database("cogniscan").Collection("question_answers")
}

// GetNotesByIDs retrieves notes by their IDs
func GetNotesByIDs(ctx context.Context, noteIDs []string) ([]models.Note, error) {
	if len(noteIDs) == 0 {
		return []models.Note{}, nil
	}

	collection := database.Client.Database("cogniscan").Collection("notes")
	objectIDs := make([]primitive.ObjectID, 0, len(noteIDs))

	for _, id := range noteIDs {
		objID, err := primitive.ObjectIDFromHex(id)
		if err != nil {
			continue
		}
		objectIDs = append(objectIDs, objID)
	}

	if len(objectIDs) == 0 {
		return []models.Note{}, nil
	}

	filter := bson.M{"_id": bson.M{"$in": objectIDs}}
	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}

	var notes []models.Note
	if err := cursor.All(ctx, &notes); err != nil {
		return nil, err
	}

	return notes, nil
}

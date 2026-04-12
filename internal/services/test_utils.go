package services

import (
	"context"
	"testing"

	"cogniscan/backend/internal/models"
)

// MockAIService is a mock implementation of AI service for testing
type MockAIService struct {
	GenerateCaptionFunc     func(imageData []byte) (string, error)
	GenerateEmbeddingFunc    func(text string, inputType string) ([]float32, error)
	GenerateQueryEmbeddingFunc func(query string) ([]float32, error)
	GenerateQuizFunc        func(captions []string, folderName string) ([]models.Question, error)
	GenerateNameSuggestionFunc func(content string, itemType string) (string, error)
	GenerateSummaryFunc     func(caption string) (string, error)
}

func (m *MockAIService) GenerateCaption(imageData []byte) (string, error) {
	if m.GenerateCaptionFunc != nil {
		return m.GenerateCaptionFunc(imageData)
	}
	return "mock caption", nil
}

func (m *MockAIService) GenerateEmbedding(text string, inputType string) ([]float32, error) {
	if m.GenerateEmbeddingFunc != nil {
		return m.GenerateEmbeddingFunc(text, inputType)
	}
	// Return a mock embedding vector of 1024 dimensions (same as actual service)
	embedding := make([]float32, 1024)
	for i := range embedding {
		embedding[i] = 0.1
	}
	return embedding, nil
}

func (m *MockAIService) GenerateQueryEmbedding(query string) ([]float32, error) {
	if m.GenerateQueryEmbeddingFunc != nil {
		return m.GenerateQueryEmbeddingFunc(query)
	}
	embedding := make([]float32, 1024)
	for i := range embedding {
		embedding[i] = 0.1
	}
	return embedding, nil
}

func (m *MockAIService) GenerateQuiz(captions []string, folderName string) ([]models.Question, error) {
	if m.GenerateQuizFunc != nil {
		return m.GenerateQuizFunc(captions, folderName)
	}
	// Return mock quiz question
	return []models.Question{
		{
			Text:        "Mock question?",
			Options:      []string{"A", "B", "C", "D"},
			CorrectOption: 0,
			Explanation:  "Mock explanation",
		},
	}, nil
}

func (m *MockAIService) GenerateNameSuggestion(content string, itemType string) (string, error) {
	if m.GenerateNameSuggestionFunc != nil {
		return m.GenerateNameSuggestionFunc(content, itemType)
	}
	return "Mock Name Suggestion", nil
}

func (m *MockAIService) GenerateSummary(caption string) (string, error) {
	if m.GenerateSummaryFunc != nil {
		return m.GenerateSummaryFunc(caption)
	}
	return "Mock summary based on content.", nil
}

// MockDriveService is a mock implementation of Drive service for testing
type MockDriveService struct {
	UploadFileFunc         func(filename string, content []byte) (string, error)
	DownloadFileContentFunc   func(fileID string) ([]byte, error)
	DeleteFileFunc          func(fileID string) error
}

func (m *MockDriveService) UploadFile(filename string, content []byte) (string, error) {
	if m.UploadFileFunc != nil {
		return m.UploadFileFunc(filename, content)
	}
	return "mock-drive-file-id", nil
}

func (m *MockDriveService) DownloadFileContent(fileID string) ([]byte, error) {
	if m.DownloadFileContentFunc != nil {
		return m.DownloadFileContentFunc(fileID)
	}
	return []byte("mock image content"), nil
}

func (m *MockDriveService) DeleteFile(fileID string) error {
	if m.DeleteFileFunc != nil {
		return m.DeleteFileFunc(fileID)
	}
	return nil
}

// MockRedisService is a mock implementation of Redis for testing
type MockRedisService struct {
	SetFunc      func(ctx context.Context, key string, value interface{}, expiration int) error
	GetFunc      func(ctx context.Context, key string) (string, error)
	DelFunc      func(ctx context.Context, keys ...string) error
	IncrFunc     func(ctx context.Context, key string) (int64, error)
	ExpireFunc   func(ctx context.Context, key string, expiration int) error
	ExistsFunc   func(ctx context.Context, keys ...string) (int64, error)
}

func (m *MockRedisService) Set(ctx context.Context, key string, value interface{}, expiration int) error {
	if m.SetFunc != nil {
		return m.SetFunc(ctx, key, value, expiration)
	}
	return nil
}

func (m *MockRedisService) Get(ctx context.Context, key string) (string, error) {
	if m.GetFunc != nil {
		return m.GetFunc(ctx, key)
	}
	return "", nil
}

func (m *MockRedisService) Del(ctx context.Context, keys ...string) error {
	if m.DelFunc != nil {
		return m.DelFunc(ctx, keys...)
	}
	return nil
}

func (m *MockRedisService) Incr(ctx context.Context, key string) (int64, error) {
	if m.IncrFunc != nil {
		return m.IncrFunc(ctx, key)
	}
	return 1, nil
}

func (m *MockRedisService) Expire(ctx context.Context, key string, expiration int) error {
	if m.ExpireFunc != nil {
		return m.ExpireFunc(ctx, key, expiration)
	}
	return nil
}

func (m *MockRedisService) Exists(ctx context.Context, keys ...string) (int64, error) {
	if m.ExistsFunc != nil {
		return m.ExistsFunc(ctx, keys...)
	}
	return 0, nil
}

// SkipIfNotAvailable skips a test if service is not configured
func SkipIfNotAvailable(t *testing.T, serviceName string, configured bool) {
	if !configured {
		t.Skipf("%s not configured, skipping test", serviceName)
	}
}

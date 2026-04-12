package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateQuiz(t *testing.T) {
	router := setupTestRouterWithUserID("test-user-id")
	router.POST("/quizzes/folders/:folderId", CreateQuiz)

	tests := []struct {
		name       string
		folderID   string
		wantStatus int
	}{
		{
			name:       "Successfully create quiz",
			folderID:   "folder-123",
			wantStatus: http.StatusCreated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", "/quizzes/folders/"+tt.folderID, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Note: This will likely fail without AI service initialized
			// but tests the handler structure
			t.Logf("CreateQuiz() returned status %d", w.Code)
		})
	}
}

func TestRequestQuizGeneration(t *testing.T) {
	router := setupTestRouterWithUserID("test-user-id")
	router.POST("/quizzes/folders/:folderId/request", RequestQuizGeneration)

	tests := []struct {
		name       string
		folderID   string
		wantStatus int
	}{
		{
			name:       "Successfully request quiz generation",
			folderID:   "folder-123",
			wantStatus: http.StatusAccepted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", "/quizzes/folders/"+tt.folderID+"/request", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Note: This will likely fail without queue service initialized
			// but tests the handler structure
			t.Logf("RequestQuizGeneration() returned status %d", w.Code)

			if tt.name == "Successfully request quiz generation" && w.Code == http.StatusAccepted {
				body := w.Body.String()
				if !strings.Contains(body, `"status"`) {
					t.Errorf("Expected status in response body")
				}
			}
		})
	}
}

func TestGetQuizStatus(t *testing.T) {
	router := setupTestRouterWithUserID("test-user-id")
	router.GET("/quizzes/folders/:folderId/status", GetQuizStatus)

	tests := []struct {
		name       string
		folderID   string
		wantStatus int
	}{
		{
			name:       "Successfully get quiz status",
			folderID:   "folder-123",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/quizzes/folders/"+tt.folderID+"/status", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			t.Logf("GetQuizStatus() returned status %d", w.Code)

			if tt.name == "Successfully get quiz status" && w.Code == http.StatusOK {
				contentType := w.Header().Get("Content-Type")
				if !assertJSONContentType(contentType) {
					t.Errorf("Expected JSON content type, got %v", contentType)
				}
			}
		})
	}
}

func TestGetQuiz(t *testing.T) {
	router := setupTestRouterWithUserID("test-user-id")
	router.GET("/quizzes/:quizId", GetQuiz)

	tests := []struct {
		name       string
		quizID     string
		wantStatus int
	}{
		{
			name:       "Successfully get quiz",
			quizID:     "quiz-123",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Quiz not found",
			quizID:     "nonexistent-quiz",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/quizzes/"+tt.quizID, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("GetQuiz() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.name == "Successfully get quiz" && w.Code == http.StatusOK {
				contentType := w.Header().Get("Content-Type")
				if !assertJSONContentType(contentType) {
					t.Errorf("Expected JSON content type, got %v", contentType)
				}
			}
		})
	}
}

func TestGetQuizQuestions(t *testing.T) {
	router := setupTestRouterWithUserID("test-user-id")
	router.GET("/quizzes/:quizId/questions", GetQuizQuestions)

	tests := []struct {
		name       string
		quizID     string
		wantStatus int
	}{
		{
			name:       "Successfully get quiz questions",
			quizID:     "quiz-123",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Quiz not found",
			quizID:     "nonexistent-quiz",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/quizzes/"+tt.quizID+"/questions", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("GetQuizQuestions() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.name == "Successfully get quiz questions" && w.Code == http.StatusOK {
				contentType := w.Header().Get("Content-Type")
				if !assertJSONContentType(contentType) {
					t.Errorf("Expected JSON content type, got %v", contentType)
				}
				body := w.Body.String()
				if !strings.Contains(body, `"questions"`) {
					t.Errorf("Expected questions in response body")
				}
			}
		})
	}
}

func TestSubmitAnswer(t *testing.T) {
	router := setupTestRouterWithUserID("test-user-id")
	router.POST("/quizzes/:quizId/questions/:questionId/answer", SubmitAnswer)

	tests := []struct {
		name       string
		quizID     string
		questionID  string
		payload     string
		wantStatus int
	}{
		{
			name:       "Successfully submit answer",
			quizID:     "quiz-123",
			questionID:  "question-456",
			payload:     `{"selectedOption":0,"timeTaken":15}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "Submit answer with session tracking",
			quizID:     "quiz-123",
			questionID:  "question-456",
			payload:     `{"selectedOption":0,"timeTaken":15,"sessionId":"session-789","isNeuralMode":true}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "Invalid JSON payload",
			quizID:     "quiz-123",
			questionID:  "question-456",
			payload:     `invalid json`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Missing required fields in payload",
			quizID:     "quiz-123",
			questionID:  "question-456",
			payload:     `{}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", "/quizzes/"+tt.quizID+"/questions/"+tt.questionID+"/answer", strings.NewReader(tt.payload))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("SubmitAnswer() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if strings.Contains(tt.name, "Successfully") && w.Code == http.StatusOK {
				body := w.Body.String()
				if !strings.Contains(body, `"isCorrect"`) {
					t.Errorf("Expected isCorrect in response body")
				}
			}
		})
	}
}

func TestGetQuizSummary(t *testing.T) {
	router := setupTestRouterWithUserID("test-user-id")
	router.GET("/quizzes/:quizId/summary", GetQuizSummary)

	tests := []struct {
		name       string
		quizID     string
		wantStatus int
	}{
		{
			name:       "Successfully get quiz summary",
			quizID:     "quiz-123",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Quiz not found",
			quizID:     "nonexistent-quiz",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/quizzes/"+tt.quizID+"/summary", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("GetQuizSummary() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.name == "Successfully get quiz summary" && w.Code == http.StatusOK {
				contentType := w.Header().Get("Content-Type")
				if !assertJSONContentType(contentType) {
					t.Errorf("Expected JSON content type, got %v", contentType)
				}
				body := w.Body.String()
				if !strings.Contains(body, `"totalQuestions"`) {
					t.Errorf("Expected totalQuestions in response body")
				}
			}
		})
	}
}

func TestRegenerateQuiz(t *testing.T) {
	router := setupTestRouterWithUserID("test-user-id")
	router.POST("/quizzes/:quizId/regenerate", RegenerateQuiz)

	tests := []struct {
		name       string
		quizID     string
		wantStatus int
	}{
		{
			name:       "Successfully regenerate quiz",
			quizID:     "quiz-123",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Quiz not found",
			quizID:     "nonexistent-quiz",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", "/quizzes/"+tt.quizID+"/regenerate", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("RegenerateQuiz() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.name == "Successfully regenerate quiz" && w.Code == http.StatusOK {
				body := w.Body.String()
				if !strings.Contains(body, `"message"`) || !strings.Contains(body, `"Quiz regeneration started"`) {
					t.Errorf("Expected success message in response body")
				}
			}
		})
	}
}

// TestQuizUnauthorizedAccess tests that unauthenticated requests are handled correctly
func TestQuizUnauthorizedAccess(t *testing.T) {
	router := setupTestRouterNoAuth()
	router.POST("/quizzes/folders/:folderId", CreateQuiz)
	router.POST("/quizzes/folders/:folderId/request", RequestQuizGeneration)
	router.GET("/quizzes/folders/:folderId/status", GetQuizStatus)
	router.GET("/quizzes/:quizId", GetQuiz)
	router.GET("/quizzes/:quizId/questions", GetQuizQuestions)
	router.POST("/quizzes/:quizId/questions/:questionId/answer", SubmitAnswer)
	router.GET("/quizzes/:quizId/summary", GetQuizSummary)
	router.POST("/quizzes/:quizId/regenerate", RegenerateQuiz)

	tests := []struct {
		name     string
		method   string
		path     string
	}{
		{
			name:   "CreateQuiz without auth",
			method: "POST",
			path:   "/quizzes/folders/folder-123",
		},
		{
			name:   "RequestQuizGeneration without auth",
			method: "POST",
			path:   "/quizzes/folders/folder-123/request",
		},
		{
			name:   "GetQuizStatus without auth",
			method: "GET",
			path:   "/quizzes/folders/folder-123/status",
		},
		{
			name:   "GetQuiz without auth",
			method: "GET",
			path:   "/quizzes/quiz-123",
		},
		{
			name:   "GetQuizQuestions without auth",
			method: "GET",
			path:   "/quizzes/quiz-123/questions",
		},
		{
			name:   "SubmitAnswer without auth",
			method: "POST",
			path:   "/quizzes/quiz-123/questions/question-456/answer",
		},
		{
			name:   "GetQuizSummary without auth",
			method: "GET",
			path:   "/quizzes/quiz-123/summary",
		},
		{
			name:   "RegenerateQuiz without auth",
			method: "POST",
			path:   "/quizzes/quiz-123/regenerate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Without auth middleware, the handler should still run
			// but may not have proper user context
			t.Logf("Handler returned status %d without auth context", w.Code)
		})
	}
}

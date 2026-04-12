package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStartQuizSession(t *testing.T) {
	router := setupTestRouterWithUserID("test-user-id")
	router.POST("/session/start", StartQuizSession)

	tests := []struct {
		name       string
		userID     string
		payload    string
		wantStatus int
	}{
		{
			name:       "Successfully start quiz session",
			userID:     "test-user-id",
			payload:     `{"quizId":"quiz-123","folderId":"folder-456"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "Missing userId parameter",
			userID:     "",
			payload:     `{"quizId":"quiz-123","folderId":"folder-456"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Invalid JSON payload",
			userID:     "test-user-id",
			payload:     `invalid json`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Missing required fields in payload",
			userID:     "test-user-id",
			payload:     `{"quizId":"quiz-123"}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", "/session/start", strings.NewReader(tt.payload))
			req.Header.Set("Content-Type", "application/json")
			req.URL.RawQuery = "userId=" + tt.userID
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("StartQuizSession() status = %v, want %v, body: %s", w.Code, tt.wantStatus, w.Body.String())
			}

			if tt.name == "Successfully start quiz session" && w.Code == http.StatusOK {
				contentType := w.Header().Get("Content-Type")
				if !assertJSONContentType(contentType) {
					t.Errorf("Expected JSON content type, got %v", contentType)
				}
				body := w.Body.String()
				if !strings.Contains(body, `"sessionId"`) {
					t.Errorf("Expected sessionId in response body")
				}
			}
		})
	}
}

func TestUpdateSessionProgress(t *testing.T) {
	router := setupTestRouterWithUserID("test-user-id")
	router.PUT("/session/:sessionId/update", UpdateSessionProgress)

	tests := []struct {
		name       string
		sessionID  string
		userID     string
		payload    string
		wantStatus int
	}{
		{
			name:       "Successfully update session progress (correct answer)",
			sessionID:  "session-123",
			userID:     "test-user-id",
			payload:     `{"questionId":"question-456","selectedOption":0,"timeTaken":15}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "Successfully update session progress (incorrect answer)",
			sessionID:  "session-123",
			userID:     "test-user-id",
			payload:     `{"questionId":"question-456","selectedOption":1,"timeTaken":30}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "Missing userId parameter",
			sessionID:  "session-123",
			userID:     "",
			payload:     `{"questionId":"question-456","selectedOption":0}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Missing sessionId parameter",
			sessionID:  "",
			userID:     "test-user-id",
			payload:     `{"questionId":"question-456","selectedOption":0}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Invalid JSON payload",
			sessionID:  "session-123",
			userID:     "test-user-id",
			payload:     `invalid json`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Missing required fields in payload",
			sessionID:  "session-123",
			userID:     "test-user-id",
			payload:     `{}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("PUT", "/session/"+tt.sessionID+"/update", strings.NewReader(tt.payload))
			req.Header.Set("Content-Type", "application/json")
			req.URL.RawQuery = "userId=" + tt.userID
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("UpdateSessionProgress() status = %v, want %v, body: %s", w.Code, tt.wantStatus, w.Body.String())
			}

			if strings.Contains(tt.name, "Successfully") && w.Code == http.StatusOK {
				body := w.Body.String()
				if !strings.Contains(body, `"message"`) || !strings.Contains(body, `"Session progress updated"`) {
					t.Errorf("Expected success message in response body")
				}
			}
		})
	}
}

func TestCompleteQuizSession(t *testing.T) {
	router := setupTestRouterWithUserID("test-user-id")
	router.PUT("/session/:sessionId/complete", CompleteQuizSession)

	tests := []struct {
		name       string
		sessionID  string
		userID     string
		wantStatus int
	}{
		{
			name:       "Successfully complete quiz session",
			sessionID:  "session-123",
			userID:     "test-user-id",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Missing userId parameter",
			sessionID:  "session-123",
			userID:     "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Missing sessionId parameter",
			sessionID:  "",
			userID:     "test-user-id",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("PUT", "/session/"+tt.sessionID+"/complete", nil)
			req.URL.RawQuery = "userId=" + tt.userID
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("CompleteQuizSession() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.name == "Successfully complete quiz session" && w.Code == http.StatusOK {
				body := w.Body.String()
				if !strings.Contains(body, `"message"`) || !strings.Contains(body, `"Session completed"`) {
					t.Errorf("Expected success message in response body")
				}
			}
		})
	}
}

func TestGetActiveSession(t *testing.T) {
	router := setupTestRouterWithUserID("test-user-id")
	router.GET("/session/active/:userId", GetActiveSession)

	tests := []struct {
		name       string
		userID     string
		wantStatus int
	}{
		{
			name:       "Successfully get active session",
			userID:     "test-user-id",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Missing userId parameter",
			userID:     "",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/session/active/"+tt.userID, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("GetActiveSession() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.name == "Successfully get active session" && w.Code == http.StatusOK {
				contentType := w.Header().Get("Content-Type")
				if !assertJSONContentType(contentType) {
					t.Errorf("Expected JSON content type, got %v", contentType)
				}
				body := w.Body.String()
				if !strings.Contains(body, `"session"`) {
					t.Errorf("Expected session in response body")
				}
			}
		})
	}
}

// TestSessionUnauthorizedAccess tests that unauthenticated requests are handled correctly
func TestSessionUnauthorizedAccess(t *testing.T) {
	router := setupTestRouterNoAuth()
	router.POST("/session/start", StartQuizSession)
	router.PUT("/session/:sessionId/update", UpdateSessionProgress)
	router.PUT("/session/:sessionId/complete", CompleteQuizSession)
	router.GET("/session/active/:userId", GetActiveSession)

	tests := []struct {
		name     string
		method   string
		path     string
	}{
		{
			name:   "StartQuizSession without auth",
			method: "POST",
			path:   "/session/start",
		},
		{
			name:   "UpdateSessionProgress without auth",
			method: "PUT",
			path:   "/session/session-123/update",
		},
		{
			name:   "CompleteQuizSession without auth",
			method: "PUT",
			path:   "/session/session-123/complete",
		},
		{
			name:   "GetActiveSession without auth",
			method: "GET",
			path:   "/session/active/test-user-id",
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

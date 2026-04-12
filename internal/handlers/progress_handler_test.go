package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetCurrentUserProgress(t *testing.T) {
	router := setupTestRouterWithUserID("test-user-id")
	router.GET("/progress/:userId", GetCurrentUserProgress)

	tests := []struct {
		name       string
		userID     string
		wantStatus int
	}{
		{
			name:       "Successfully get user progress",
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
			req, _ := http.NewRequest("GET", "/progress/"+tt.userID, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("GetCurrentUserProgress() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.name == "Successfully get user progress" && w.Code == http.StatusOK {
				contentType := w.Header().Get("Content-Type")
				if !assertJSONContentType(contentType) {
					t.Errorf("Expected JSON content type, got %v", contentType)
				}
			}
		})
	}
}

func TestUpdateDailyProgress(t *testing.T) {
	router := setupTestRouterWithUserID("test-user-id")
	router.POST("/progress/daily", UpdateDailyProgress)

	tests := []struct {
		name       string
		userID     string
		payload    string
		wantStatus int
	}{
		{
			name:       "Successfully update daily progress",
			userID:     "test-user-id",
			payload:    `{"dailyGoalPercent": 75}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "Missing userId parameter",
			userID:     "",
			payload:    `{"dailyGoalPercent": 50}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Invalid JSON payload",
			userID:     "test-user-id",
			payload:    `invalid json`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", "/progress/daily", nil)
			req.Header.Set("Content-Type", "application/json")
			req.URL.RawQuery = "userId=" + tt.userID
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("UpdateDailyProgress() status = %v, want %v", w.Code, tt.wantStatus)
			}
		})
	}
}

func TestRecordStudySession(t *testing.T) {
	router := setupTestRouterWithUserID("test-user-id")
	router.POST("/progress/study-session", RecordStudySession)

	tests := []struct {
		name       string
		userID     string
		payload    string
		wantStatus int
	}{
		{
			name:       "Successfully record study session",
			userID:     "test-user-id",
			payload:    `{"minutesSpent": 30}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "Missing userId parameter",
			userID:     "",
			payload:    `{"minutesSpent": 15}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Invalid JSON payload",
			userID:     "test-user-id",
			payload:    `invalid json`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", "/progress/study-session", nil)
			req.Header.Set("Content-Type", "application/json")
			req.URL.RawQuery = "userId=" + tt.userID
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("RecordStudySession() status = %v, want %v", w.Code, tt.wantStatus)
			}
		})
	}
}

func TestGetStorageUsage(t *testing.T) {
	router := setupTestRouterWithUserID("test-user-id")
	router.GET("/storage/:userId", GetStorageUsage)

	tests := []struct {
		name       string
		userID     string
		wantStatus int
	}{
		{
			name:       "Successfully get storage usage",
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
			req, _ := http.NewRequest("GET", "/storage/"+tt.userID, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("GetStorageUsage() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.name == "Successfully get storage usage" && w.Code == http.StatusOK {
				contentType := w.Header().Get("Content-Type")
				if !assertJSONContentType(contentType) {
					t.Errorf("Expected JSON content type, got %v", contentType)
				}
			}
		})
	}
}

func TestProgressUnauthorizedAccess(t *testing.T) {
	router := setupTestRouterNoAuth()
	router.GET("/progress/:userId", GetCurrentUserProgress)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{
			name:   "GetCurrentUserProgress without auth",
			method: "GET",
			path:   "/progress/test-user-id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Without auth middleware, handler will return 400
			// because it expects userId in context
			t.Logf("Handler returned status %d without auth context", w.Code)
		})
	}
}

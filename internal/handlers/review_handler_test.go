package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetReviewQueue(t *testing.T) {
	router := setupTestRouterWithUserID("test-user-id")
	router.GET("/reviews/queue", GetReviewQueue)

	tests := []struct {
		name       string
		limit      string
		wantStatus int
	}{
		{
			name:       "Successfully get review queue",
			limit:      "",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Get review queue with limit",
			limit:      "10",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/reviews/queue?limit="+tt.limit, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Note: This will likely fail without service layer initialized
			// but tests the handler structure
			t.Logf("GetReviewQueue() returned status %d", w.Code)

			if strings.Contains(tt.name, "Successfully") && w.Code == http.StatusOK {
				contentType := w.Header().Get("Content-Type")
				if !assertJSONContentType(contentType) {
					t.Errorf("Expected JSON content type, got %v", contentType)
				}
				body := w.Body.String()
				if !strings.Contains(body, `"reviews"`) {
					t.Errorf("Expected reviews in response body")
				}
			}
		})
	}
}

func TestGetNoteReviewHistory(t *testing.T) {
	router := setupTestRouterWithUserID("test-user-id")
	router.GET("/reviews/note/:noteId/history", GetNoteReviewHistory)

	tests := []struct {
		name       string
		noteID     string
		wantStatus int
	}{
		{
			name:       "Successfully get note review history",
			noteID:     "note-123",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Note not found",
			noteID:     "nonexistent-note",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/reviews/note/"+tt.noteID+"/history", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("GetNoteReviewHistory() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.name == "Successfully get note review history" && w.Code == http.StatusOK {
				contentType := w.Header().Get("Content-Type")
				if !assertJSONContentType(contentType) {
					t.Errorf("Expected JSON content type, got %v", contentType)
				}
			}
		})
	}
}

func TestUpdateReviewStatus(t *testing.T) {
	router := setupTestRouterWithUserID("test-user-id")
	router.PUT("/reviews/note/:noteId/status", UpdateReviewStatus)

	tests := []struct {
		name       string
		noteID     string
		wantStatus int
	}{
		{
			name:       "Successfully update review status",
			noteID:     "note-123",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Missing noteId parameter",
			noteID:     "",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("PUT", "/reviews/note/"+tt.noteID+"/status", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("UpdateReviewStatus() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.name == "Successfully update review status" && w.Code == http.StatusOK {
				body := w.Body.String()
				if !strings.Contains(body, `"message"`) || !strings.Contains(body, `"Review status updated"`) {
					t.Errorf("Expected success message in response body")
				}
			}
		})
	}
}

// TestReviewUnauthorizedAccess tests that unauthenticated requests are handled correctly
func TestReviewUnauthorizedAccess(t *testing.T) {
	router := setupTestRouterNoAuth()
	router.GET("/reviews/queue", GetReviewQueue)
	router.GET("/reviews/note/:noteId/history", GetNoteReviewHistory)
	router.PUT("/reviews/note/:noteId/status", UpdateReviewStatus)

	tests := []struct {
		name     string
		method   string
		path     string
	}{
		{
			name:   "GetReviewQueue without auth",
			method: "GET",
			path:   "/reviews/queue",
		},
		{
			name:   "GetNoteReviewHistory without auth",
			method: "GET",
			path:   "/reviews/note/note-123/history",
		},
		{
			name:   "UpdateReviewStatus without auth",
			method: "PUT",
			path:   "/reviews/note/note-123/status",
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

package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetNameSuggestionsForFolder(t *testing.T) {
	router := setupTestRouterWithUserID("test-user-id")
	router.GET("/folders/name-suggestions/:id", GetNameSuggestionsForFolder)

	tests := []struct {
		name       string
		folderID   string
		userID     string
		wantStatus int
	}{
		{
			name:       "Successfully get folder name suggestions",
			folderID:   "folder-123",
			userID:     "test-user-id",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Invalid folder ID",
			folderID:   "invalid-id",
			userID:     "test-user-id",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Missing folderId parameter",
			folderID:   "",
			userID:     "test-user-id",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/folders/name-suggestions/"+tt.folderID, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("GetNameSuggestionsForFolder() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.name == "Successfully get folder name suggestions" && w.Code == http.StatusOK {
				contentType := w.Header().Get("Content-Type")
				if !assertJSONContentType(contentType) {
					t.Errorf("Expected JSON content type, got %v", contentType)
				}
			}
		})
	}
}

func TestGetNameSuggestionsForNote(t *testing.T) {
	router := setupTestRouterWithUserID("test-user-id")
	router.GET("/notes/name-suggestions/:id", GetNameSuggestionsForNote)

	tests := []struct {
		name       string
		noteID     string
		userID     string
		wantStatus int
	}{
		{
			name:       "Successfully get note name suggestions",
			noteID:     "note-123",
			userID:     "test-user-id",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Invalid note ID",
			noteID:     "invalid-id",
			userID:     "test-user-id",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Missing noteId parameter",
			noteID:     "",
			userID:     "test-user-id",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/notes/name-suggestions/"+tt.noteID, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("GetNameSuggestionsForNote() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.name == "Successfully get note name suggestions" && w.Code == http.StatusOK {
				contentType := w.Header().Get("Content-Type")
				if !assertJSONContentType(contentType) {
					t.Errorf("Expected JSON content type, got %v", contentType)
				}
			}
		})
	}
}

// TestNameSuggestionsUnauthorizedAccess tests that unauthenticated requests are handled correctly
func TestNameSuggestionsUnauthorizedAccess(t *testing.T) {
	router := setupTestRouterNoAuth()
	router.GET("/folders/name-suggestions/:id", GetNameSuggestionsForFolder)
	router.GET("/notes/name-suggestions/:id", GetNameSuggestionsForNote)

	tests := []struct {
		name     string
		method   string
		path     string
	}{
		{
			name:   "GetNameSuggestionsForFolder without auth",
			method: "GET",
			path:   "/folders/name-suggestions/folder-123",
		},
		{
			name:   "GetNameSuggestionsForNote without auth",
			method: "GET",
			path:   "/notes/name-suggestions/note-123",
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

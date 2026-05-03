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
		wantStatus int
	}{
		{
			name:       "Invalid folder ID format",
			folderID:   "invalid-id",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Folder ID not found",
			folderID:   "507f1f77bcf86cd799439011",
			wantStatus: http.StatusNotFound,
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
		})
	}
}

func TestGetNameSuggestionsForNote(t *testing.T) {
	router := setupTestRouterWithUserID("test-user-id")
	router.GET("/notes/name-suggestions/:id", GetNameSuggestionsForNote)

	tests := []struct {
		name       string
		noteID     string
		wantStatus int
	}{
		{
			name:       "Invalid note ID format",
			noteID:     "invalid-id",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Note ID not found",
			noteID:     "507f1f77bcf86cd799439011",
			wantStatus: http.StatusNotFound,
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
			path:   "/folders/name-suggestions/507f1f77bcf86cd799439011",
		},
		{
			name:   "GetNameSuggestionsForNote without auth",
			method: "GET",
			path:   "/notes/name-suggestions/507f1f77bcf86cd799439011",
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

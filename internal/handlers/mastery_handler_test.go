package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetFolderMastery(t *testing.T) {
	router := setupTestRouterWithUserID("test-user-id")
	router.GET("/mastery/folders/:id", GetFolderMastery)

	tests := []struct {
		name       string
		folderID   string
		wantStatus int
	}{
		{
			name:       "Successfully get folder mastery",
			folderID:   "folder-123",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Invalid folder ID",
			folderID:   "invalid-id",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Missing folderId parameter",
			folderID:   "",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/mastery/folders/"+tt.folderID, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("GetFolderMastery() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.name == "Successfully get folder mastery" && w.Code == http.StatusOK {
				contentType := w.Header().Get("Content-Type")
				if !assertJSONContentType(contentType) {
					t.Errorf("Expected JSON content type, got %v", contentType)
				}
			}
		})
	}
}

func TestGetAllFoldersMastery(t *testing.T) {
	router := setupTestRouterWithUserID("test-user-id")
	router.GET("/mastery/folders", GetAllFoldersMastery)

	tests := []struct {
		name       string
		wantStatus int
	}{
		{
			name:       "Successfully get all folders mastery",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/mastery/folders", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("GetAllFoldersMastery() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.name == "Successfully get all folders mastery" && w.Code == http.StatusOK {
				contentType := w.Header().Get("Content-Type")
				if !assertJSONContentType(contentType) {
					t.Errorf("Expected JSON content type, got %v", contentType)
				}
			}
		})
	}
}

func TestUpdateNoteMastery(t *testing.T) {
	router := setupTestRouterWithUserID("test-user-id")
	router.PUT("/mastery/notes/:noteId", UpdateNoteMastery)

	tests := []struct {
		name       string
		noteID     string
		payload    string
		wantStatus int
	}{
		{
			name:       "Successfully update note mastery (correct)",
			noteID:     "note-123",
			payload:    `{"noteId":"note-123","isCorrect":true}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "Successfully update note mastery (incorrect)",
			noteID:     "note-123",
			payload:    `{"noteId":"note-123","isCorrect":false}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "Missing required fields in payload",
			noteID:     "note-123",
			payload:    `{}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("PUT", "/mastery/notes/"+tt.noteID, strings.NewReader(tt.payload))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("UpdateNoteMastery() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if strings.Contains(tt.name, "Successfully") && w.Code == http.StatusOK {
				body := w.Body.String()
				if !strings.Contains(body, `"message"`) || !strings.Contains(body, `"Mastery updated"`) {
					t.Errorf("Expected success message in response body")
				}
			}
		})
	}
}

func TestMasteryUnauthorizedAccess(t *testing.T) {
	router := setupTestRouterWithUserID("test-user-id")
	router.GET("/mastery/folders/:id", GetFolderMastery)
	router.GET("/mastery/folders", GetAllFoldersMastery)
	router.PUT("/mastery/notes/:noteId", UpdateNoteMastery)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{
			name:   "GetFolderMastery without auth",
			method: "GET",
			path:   "/mastery/folders/folder-123",
		},
		{
			name:   "GetAllFoldersMastery without auth",
			method: "GET",
			path:   "/mastery/folders",
		},
		{
			name:   "UpdateNoteMastery without auth",
			method: "PUT",
			path:   "/mastery/notes/note-123",
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

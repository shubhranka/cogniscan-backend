package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetNoteIndexStatus(t *testing.T) {
	router := setupTestRouterWithUserID("test-user-id")
	router.GET("/indexing/notes/:id", GetNoteIndexStatus)

	tests := []struct {
		name       string
		noteID     string
		wantStatus int
	}{
		{
			name:       "Successfully get note index status",
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
			req, _ := http.NewRequest("GET", "/indexing/notes/"+tt.noteID, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("GetNoteIndexStatus() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.name == "Successfully get note index status" && w.Code == http.StatusOK {
				contentType := w.Header().Get("Content-Type")
				if !assertJSONContentType(contentType) {
					t.Errorf("Expected JSON content type, got %v", contentType)
				}
			}
		})
	}
}

func TestGetFolderIndexStatus(t *testing.T) {
	router := setupTestRouterWithUserID("test-user-id")
	router.GET("/indexing/folders/:id", GetFolderIndexStatus)

	tests := []struct {
		name       string
		folderID   string
		wantStatus int
	}{
		{
			name:       "Successfully get folder index status",
			folderID:   "folder-123",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Missing folderId parameter",
			folderID:   "",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/indexing/folders/"+tt.folderID, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("GetFolderIndexStatus() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.name == "Successfully get folder index status" && w.Code == http.StatusOK {
				contentType := w.Header().Get("Content-Type")
				if !assertJSONContentType(contentType) {
					t.Errorf("Expected JSON content type, got %v", contentType)
				}
			}
		})
	}
}

func TestUpdateDocumentIndex(t *testing.T) {
	router := setupTestRouterWithUserID("test-user-id")
	router.PUT("/indexing/notes/:noteId", UpdateDocumentIndex)

	tests := []struct {
		name       string
		noteID     string
		payload    string
		wantStatus int
	}{
		{
			name:       "Successfully update document index",
			noteID:     "note-123",
			payload:    `{"noteId":"note-123","folderId":"folder-456","indexStatus":"completed","pagesIndexed":5,"totalPages":5}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "Missing noteId parameter",
			noteID:     "",
			payload:    `{"folderId":"folder-456"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Invalid JSON payload",
			noteID:     "note-123",
			payload:    `invalid json`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("PUT", "/indexing/notes/"+tt.noteID, strings.NewReader(tt.payload))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("UpdateDocumentIndex() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.name == "Successfully update document index" && w.Code == http.StatusOK {
				body := w.Body.String()
				if !strings.Contains(body, `"message"`) || !strings.Contains(body, `"Document index updated"`) {
					t.Errorf("Expected success message in response body")
				}
			}
		})
	}
}

func TestGenerateSummary(t *testing.T) {
	router := setupTestRouterWithUserID("test-user-id")
	router.POST("/indexing/notes/:noteId/summary", GenerateSummary)

	tests := []struct {
		name       string
		noteID     string
		payload    string
		wantStatus int
	}{
		{
			name:       "Successfully generate summary",
			noteID:     "note-123",
			payload:    `{"noteId":"note-123"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "Missing noteId parameter",
			noteID:     "",
			payload:    `{}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Invalid JSON payload",
			noteID:     "note-123",
			payload:    `invalid json`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", "/indexing/notes/"+tt.noteID+"/summary", strings.NewReader(tt.payload))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("GenerateSummary() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.name == "Successfully generate summary" && w.Code == http.StatusOK {
				body := w.Body.String()
				if !strings.Contains(body, `"message"`) || !strings.Contains(body, `"Summary generation completed"`) {
					t.Errorf("Expected success message in response body")
				}
			}
		})
	}
}

func TestIndexingUnauthorizedAccess(t *testing.T) {
	router := setupTestRouterWithUserID("test-user-id")
	router.GET("/indexing/notes/:id", GetNoteIndexStatus)
	router.GET("/indexing/folders/:id", GetFolderIndexStatus)
	router.PUT("/indexing/notes/:noteId", UpdateDocumentIndex)
	router.POST("/indexing/notes/:noteId/summary", GenerateSummary)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{
			name:   "GetNoteIndexStatus without auth",
			method: "GET",
			path:   "/indexing/notes/note-123",
		},
		{
			name:   "GetFolderIndexStatus without auth",
			method: "GET",
			path:   "/indexing/folders/folder-123",
		},
		{
			name:   "UpdateDocumentIndex without auth",
			method: "PUT",
			path:   "/indexing/notes/note-123",
		},
		{
			name:   "GenerateSummary without auth",
			method: "POST",
			path:   "/indexing/notes/note-123/summary",
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

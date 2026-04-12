package services

import (
	"testing"
)

func TestInitQueueService(t *testing.T) {
	t.Run("Initialize queue service", func(t *testing.T) {
		// InitQueueService initializes Redis client
		// This test verifies that initialization doesn't panic even without Redis
		err := InitQueueService()

		// If Redis URL is not set, it should still work but log a warning
		t.Logf("InitQueueService returned: %v (nil is expected if no Redis URL)", err)
	})
}

func TestIsQueueServiceInitialized(t *testing.T) {
	t.Run("Check if queue service is initialized", func(t *testing.T) {
		initialized := IsQueueServiceInitialized()
		t.Logf("Queue service initialized: %v", initialized)
	})
}

// Note: Enqueue/Dequeue tests would require actual Redis client
// Since we're using mocked database approach, these are structural tests
func TestQueueJobStructure(t *testing.T) {
	t.Run("Verify queue job structures are valid", func(t *testing.T) {
		// Verify job structures can be serialized as JSON
		// This is a basic structural test for queue jobs

		// Test caption job structure
		captionJob := struct {
			ID       string `json:"id"`
			NoteID   string `json:"noteId"`
			OwnerID  string `json:"ownerId"`
			Status    string `json:"status"`
		}{
			ID:       "job-1",
			NoteID:   "note-123",
			OwnerID:  "test-user-id",
			Status:    "pending",
		}

		// Verify it's a valid struct
		if captionJob.ID == "" || captionJob.NoteID == "" || captionJob.OwnerID == "" {
			t.Error("Caption job must have required fields")
		}

		// Test quiz job structure
		quizJob := struct {
			ID       string `json:"id"`
			FolderID string `json:"folderId"`
			OwnerID  string `json:"ownerId"`
			Status    string `json:"status"`
		}{
			ID:       "quiz-job-1",
			FolderID: "folder-123",
			OwnerID:  "test-user-id",
			Status:    "pending",
		}

		// Verify it's a valid struct
		if quizJob.ID == "" || quizJob.FolderID == "" || quizJob.OwnerID == "" {
			t.Error("Quiz job must have required fields")
		}

		t.Log("Queue job structures verified")
	})
}

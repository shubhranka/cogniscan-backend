package queue

// CaptionJob represents a caption generation job in the queue
type CaptionJob struct {
	ID      string `json:"id"`      // Unique job ID
	NoteID  string `json:"noteId"`  // MongoDB note ID
	DriveID string `json:"driveId"` // Google Drive file ID
}

// QuizJob represents a quiz generation job in the queue
type QuizJob struct {
	ID       string `json:"id"`       // Unique job ID
	FolderID string `json:"folderId"` // Folder ID to generate quiz for
	OwnerID  string `json:"ownerId"`  // User ID who requested the quiz
}

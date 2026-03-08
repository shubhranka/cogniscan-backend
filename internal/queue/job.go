package queue

// CaptionJob represents a caption generation job in the queue
type CaptionJob struct {
	ID      string `json:"id"`      // Unique job ID
	NoteID  string `json:"noteId"`  // MongoDB note ID
	DriveID string `json:"driveId"` // Google Drive file ID
}

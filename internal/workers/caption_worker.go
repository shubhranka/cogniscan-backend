package workers

import (
	"context"
	"io"
	"log"
	"os"
	"strconv"
	"time"

	"cogniscan/backend/internal/database"
	"cogniscan/backend/internal/queue"
	"cogniscan/backend/internal/services"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	maxRetryDefault = 3
	workerTimeout  = 30 * time.Second
)

// StartCaptionWorker starts the caption generation worker pool
func StartCaptionWorker(ctx context.Context, workerCount int) {
	if !services.IsQueueServiceInitialized() {
		log.Println("[CaptionWorker] Queue service not initialized, worker not started")
		return
	}

	if workerCount <= 0 {
		workerCount = 3 // Default worker count
	}

	// Read max retry from env or use default
	maxRetry := maxRetryDefault
	if maxRetryStr := os.Getenv("CAPTION_MAX_RETRY"); maxRetryStr != "" {
		if n, err := strconv.Atoi(maxRetryStr); err == nil && n > 0 {
			maxRetry = n
		}
	}

	log.Printf("[CaptionWorker] Starting %d workers with max retries: %d", workerCount, maxRetry)

	// Start worker goroutines (run in background)
	for i := 0; i < workerCount; i++ {
		go worker(ctx, i, maxRetry)
	}

	log.Println("[CaptionWorker] Workers started in background")
}

// worker is an individual worker goroutine that processes caption jobs
func worker(ctx context.Context, id int, maxRetry int) {
	log.Printf("[CaptionWorker-%d] Started", id)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[CaptionWorker-%d] Exiting", id)
			return
		default:
			// Dequeue next job with timeout
			job, err := services.DequeueCaptionJob(workerTimeout)
			if err != nil {
				log.Printf("[CaptionWorker-%d] Error dequeuing: %v", id, err)
				time.Sleep(5 * time.Second)
				continue
			}

			if job == nil {
				// No jobs available, sleep briefly
				time.Sleep(5 * time.Second)
				continue
			}

			log.Printf("[CaptionWorker-%d] Processing job %s for note %s", id, job.ID, job.NoteID)

			// Process the job with retry logic
			if err := processJobWithRetry(ctx, job, maxRetry); err != nil {
				log.Printf("[CaptionWorker-%d] Job %s failed after %d retries: %v", id, job.ID, maxRetry, err)
			} else {
				log.Printf("[CaptionWorker-%d] Job %s completed successfully", id, job.ID)
			}
		}
	}
}

// processJobWithRetry processes a single job with retry logic
func processJobWithRetry(ctx context.Context, job *queue.CaptionJob, maxRetry int) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetry; attempt++ {
		if attempt > 0 {
			// Exponential backoff before retry
			backoff := time.Duration(1<<uint(attempt)) * time.Second
			log.Printf("[CaptionWorker] Retry %d/%d for job %s after %v", attempt, maxRetry, job.ID, backoff)
			time.Sleep(backoff)
		}

		// Update status to "processing" (only on first attempt)
		if attempt == 0 {
			if err := updateNoteStatus(job.NoteID, "processing", ""); err != nil {
				return err
			}
		}

		// Process the job
		if err := processCaptionJob(ctx, job); err != nil {
			lastErr = err
			log.Printf("[CaptionWorker] Job %s attempt %d failed: %v", job.ID, attempt+1, err)
			continue
		}

		// Success - update status to "completed"
		return updateNoteStatus(job.NoteID, "completed", "")
	}

	// All retries failed - mark as "failed"
	return updateNoteStatus(job.NoteID, "failed", lastErr.Error())
}

// processCaptionJob processes a single caption job
func processCaptionJob(ctx context.Context, job *queue.CaptionJob) error {
	// Download image from Drive
	resp, err := services.DownloadFileContent(job.DriveID)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Read image bytes
	imageBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Generate caption using AI service
	caption, err := services.GenerateCaption(imageBytes)
	if err != nil {
		return err
	}

	// Update MongoDB with caption
	notesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("notes")
	noteCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	objID, err := primitive.ObjectIDFromHex(job.NoteID)
	if err != nil {
		return err
	}

	filter := bson.M{"_id": objID}
	update := bson.M{"$set": bson.M{"caption": caption, "updatedAt": time.Now()}}

	result, err := notesCollection.UpdateOne(noteCtx, filter, update)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return nil // Note might have been deleted, not an error
	}

	// Fetch the note to get folderId and ownerId for vector storage
	var note bson.M
	if err := notesCollection.FindOne(noteCtx, filter).Decode(&note); err == nil {
		// Generate embedding for the caption
		vector, err := services.GenerateEmbedding(caption)
		if err != nil {
			log.Printf("[CaptionWorker] Failed to generate embedding for note %s: %v", job.NoteID, err)
		} else {
			// Store the embedding in the vector collection
			folderID, _ := note["folderId"].(string)
			ownerID, _ := note["ownerId"].(string)

			if err := services.StoreCaptionEmbedding(job.NoteID, folderID, ownerID, caption, vector); err != nil {
				log.Printf("[CaptionWorker] Failed to store embedding for note %s: %v", job.NoteID, err)
			} else {
				log.Printf("[CaptionWorker] Stored embedding for note %s", job.NoteID)
			}
		}
	}

	log.Printf("[CaptionWorker] Generated and saved transcription for note %s", job.NoteID)
	return nil
}

// updateNoteStatus updates the caption status of a note in MongoDB
func updateNoteStatus(noteID string, status string, errorMsg string) error {
	notesCollection := database.Client.Database(os.Getenv("DB_NAME")).Collection("notes")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	objID, err := primitive.ObjectIDFromHex(noteID)
	if err != nil {
		return err
	}

	update := bson.M{"$set": bson.M{"captionStatus": status, "updatedAt": time.Now()}}
	if errorMsg != "" {
		update["$set"].(bson.M)["captionError"] = errorMsg
	} else {
		// Clear error on success
		update["$unset"] = bson.M{"captionError": ""}
	}

	filter := bson.M{"_id": objID}
	result, err := notesCollection.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return nil // Note might have been deleted, not an error
	}

	log.Printf("[CaptionWorker] Updated note %s status to %s", noteID, status)
	return nil
}

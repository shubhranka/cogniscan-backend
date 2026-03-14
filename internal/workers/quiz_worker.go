package workers

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"cogniscan/backend/internal/queue"
	"cogniscan/backend/internal/services"
)

const (
	quizMaxRetryDefault = 3
	quizWorkerTimeout   = 30 * time.Second
)

// StartQuizWorker starts the quiz generation worker pool
func StartQuizWorker(ctx context.Context, workerCount int) {
	if !services.IsQueueServiceInitialized() {
		log.Println("[QuizWorker] Queue service not initialized, worker not started")
		return
	}

	if workerCount <= 0 {
		workerCount = 2 // Default worker count (quiz jobs may be longer)
	}

	// Read max retry from env or use default
	maxRetry := quizMaxRetryDefault
	if maxRetryStr := os.Getenv("QUIZ_MAX_RETRY"); maxRetryStr != "" {
		if n, err := strconv.Atoi(maxRetryStr); err == nil && n > 0 {
			maxRetry = n
		}
	}

	log.Printf("[QuizWorker] Starting %d workers with max retries: %d", workerCount, maxRetry)

	// Start worker goroutines (run in background)
	for i := 0; i < workerCount; i++ {
		go quizWorker(ctx, i, maxRetry)
	}

	log.Println("[QuizWorker] Workers started in background")
}

// quizWorker is an individual worker goroutine that processes quiz jobs
func quizWorker(ctx context.Context, id int, maxRetry int) {
	log.Printf("[QuizWorker-%d] Started", id)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[QuizWorker-%d] Exiting", id)
			return
		default:
			// Dequeue next job with timeout
			job, err := services.DequeueQuizJob(quizWorkerTimeout)
			if err != nil {
				log.Printf("[QuizWorker-%d] Error dequeuing: %v", id, err)
				time.Sleep(5 * time.Second)
				continue
			}

			if job == nil {
				// No jobs available, sleep briefly
				time.Sleep(5 * time.Second)
				continue
			}

			log.Printf("[QuizWorker-%d] Processing job %s for folder %s", id, job.ID, job.FolderID)

			// Process the job with retry logic
			if err := processQuizJobWithRetry(ctx, job, maxRetry); err != nil {
				log.Printf("[QuizWorker-%d] Job %s failed after %d retries: %v", id, job.ID, maxRetry, err)
			} else {
				log.Printf("[QuizWorker-%d] Job %s completed successfully", id, job.ID)
			}
		}
	}
}

// processQuizJobWithRetry processes a single quiz job with retry logic
func processQuizJobWithRetry(ctx context.Context, job *queue.QuizJob, maxRetry int) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetry; attempt++ {
		if attempt > 0 {
			// Exponential backoff before retry
			backoff := time.Duration(1<<uint(attempt)) * time.Second
			log.Printf("[QuizWorker] Retry %d/%d for job %s after %v", attempt, maxRetry, job.ID, backoff)
			time.Sleep(backoff)
		}

		// Process the job
		_, _, err := services.CreateQuizForFolder(ctx, job.FolderID, job.OwnerID, true)
		if err != nil {
			lastErr = err
			log.Printf("[QuizWorker] Job %s attempt %d failed: %v", job.ID, attempt+1, err)
			continue
		}

		// Success - status already updated by CreateQuizForFolder
		return nil
	}

	// All retries failed - status already updated by CreateQuizForFolder
	return lastErr
}

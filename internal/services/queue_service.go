package services

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	"cogniscan/backend/internal/queue"

	"github.com/redis/go-redis/v9"
)

var redisClient *redis.Client
const (
	queueKey       = "cogniscan:caption:queue"
	quizQueueKey   = "cogniscan:quiz:queue"
	queueTTL       = 24 * time.Hour
	workerTTL      = 30 * time.Second
)

// InitQueueService initializes the Redis client with Upstash
func InitQueueService() error {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		log.Println("[QueueService] REDIS_URL not set, queue service disabled")
		return nil
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return err
	}

	redisClient = redis.NewClient(opt)

	// Set TTL on the queue key to prevent indefinite growth
	if err := redisClient.Expire(context.Background(), queueKey, queueTTL).Err(); err != nil {
		log.Printf("[QueueService] Warning: Failed to set queue TTL: %v", err)
	}

	log.Println("[QueueService] Successfully initialized Redis queue service")
	return nil
}

// EnqueueCaptionJob adds a caption generation job to the Redis queue
func EnqueueCaptionJob(job queue.CaptionJob) error {
	if redisClient == nil {
		return nil // Queue service disabled
	}

	jobJSON, err := json.Marshal(job)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := redisClient.RPush(ctx, queueKey, jobJSON).Err(); err != nil {
		log.Printf("[QueueService] Failed to enqueue job: %v", err)
		return err
	}

	// Refresh TTL when new jobs are added
	if err := redisClient.Expire(ctx, queueKey, queueTTL).Err(); err != nil {
		log.Printf("[QueueService] Warning: Failed to refresh queue TTL: %v", err)
	}

	log.Printf("[QueueService] Enqueued job %s for note %s", job.ID, job.NoteID)
	return nil
}

// DequeueCaptionJob gets the next job from the queue (blocking with timeout)
func DequeueCaptionJob(timeout time.Duration) (*queue.CaptionJob, error) {
	if redisClient == nil {
		return nil, nil // Queue service disabled
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// BLPOP blocks until a job is available or timeout
	result, err := redisClient.BLPop(ctx, timeout, queueKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // No jobs available (timeout)
		}
		return nil, err
	}

	if len(result) == 0 {
		return nil, nil
	}

	var job queue.CaptionJob
	if err := json.Unmarshal([]byte(result[1]), &job); err != nil {
		log.Printf("[QueueService] Failed to unmarshal job: %v", err)
		return nil, err
	}

	return &job, nil
}

// IsQueueServiceInitialized checks if the queue service is initialized
func IsQueueServiceInitialized() bool {
	return redisClient != nil
}

// EnqueueQuizJob adds a quiz generation job to Redis queue
func EnqueueQuizJob(job queue.QuizJob) error {
	if redisClient == nil {
		return nil // Queue service disabled
	}

	jobJSON, err := json.Marshal(job)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := redisClient.RPush(ctx, quizQueueKey, jobJSON).Err(); err != nil {
		log.Printf("[QueueService] Failed to enqueue quiz job: %v", err)
		return err
	}

	// Refresh TTL when new jobs are added
	if err := redisClient.Expire(ctx, quizQueueKey, queueTTL).Err(); err != nil {
		log.Printf("[QueueService] Warning: Failed to refresh quiz queue TTL: %v", err)
	}

	log.Printf("[QueueService] Enqueued quiz job %s for folder %s", job.ID, job.FolderID)
	return nil
}

// DequeueQuizJob gets the next quiz job from the queue (blocking with timeout)
func DequeueQuizJob(timeout time.Duration) (*queue.QuizJob, error) {
	if redisClient == nil {
		return nil, nil // Queue service disabled
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// BLPOP blocks until a job is available or timeout
	result, err := redisClient.BLPop(ctx, timeout, quizQueueKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // No jobs available (timeout)
		}
		return nil, err
	}

	if len(result) == 0 {
		return nil, nil
	}

	var job queue.QuizJob
	if err := json.Unmarshal([]byte(result[1]), &job); err != nil {
		log.Printf("[QueueService] Failed to unmarshal quiz job: %v", err)
		return nil, err
	}

	return &job, nil
}

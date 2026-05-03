package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"cogniscan/backend/internal/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var (
	ErrQueueShutdown = errors.New("queue is shutting down")
	ErrJobNotFound   = errors.New("job not found")
)

// MasteryQueue manages background mastery update jobs
type MasteryQueue struct {
	jobs      chan *MasteryUpdateJob
	workers   int
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
	started   bool
	shutdown  bool
	db        interface{} // Will be replaced with actual database client
}

// MasteryUpdateJob represents a mastery update job
type MasteryUpdateJob struct {
	ID     string `json:"id"`
	NodeID string `json:"nodeId"` // The node that triggered the update
}

// NewMasteryQueue creates a new mastery update queue
func NewMasteryQueue(workers int, db interface{}) *MasteryQueue {
	ctx, cancel := context.WithCancel(context.Background())
	return &MasteryQueue{
		jobs:    make(chan *MasteryUpdateJob, 1000),
		workers: workers,
		ctx:     ctx,
		cancel:  cancel,
		db:      db,
	}
}

// Start begins processing jobs
func (q *MasteryQueue) Start() {
	if q.started {
		return
	}
	q.started = true

	for i := 0; i < q.workers; i++ {
		q.wg.Add(1)
		go q.worker(i)
	}

	log.Printf("MasteryQueue started with %d workers", q.workers)
}

// Stop gracefully shuts down the queue
func (q *MasteryQueue) Stop() {
	if !q.started || q.shutdown {
		return
	}
	q.shutdown = true

	log.Println("MasteryQueue shutting down...")

	// Close the jobs channel to signal workers to stop
	close(q.jobs)

	// Wait for all workers to finish
	done := make(chan struct{})
	go func() {
		q.wg.Wait()
		close(done)
	}()

	// Wait up to 30 seconds for workers to finish
	select {
	case <-done:
		log.Println("MasteryQueue stopped gracefully")
	case <-time.After(30 * time.Second):
		log.Println("MasteryQueue stopped (timeout)")
	}
}

// Enqueue adds a job to the queue
func (q *MasteryQueue) Enqueue(job *MasteryUpdateJob) error {
	if q.shutdown {
		return ErrQueueShutdown
	}

	select {
	case q.jobs <- job:
		return nil
	default:
		return errors.New("queue is full")
	}
}

// worker processes jobs from the queue
func (q *MasteryQueue) worker(id int) {
	defer q.wg.Done()

	log.Printf("MasteryQueue worker %d started", id)

	for {
		select {
		case job, ok := <-q.jobs:
			if !ok {
				// Channel closed, exit
				log.Printf("MasteryQueue worker %d exiting", id)
				return
			}

			q.processJob(job)

		case <-q.ctx.Done():
			log.Printf("MasteryQueue worker %d cancelled", id)
			return
		}
	}
}

// processJob handles a single mastery update job
func (q *MasteryQueue) processJob(job *MasteryUpdateJob) {
	log.Printf("Processing mastery update job for node %s", job.NodeID)

	// TODO: Implement actual mastery propagation logic
	// Call services.UpdateAncestorMasteryAsync from the mastery service
	// This requires a processor function injection to avoid circular imports

	log.Printf("Mastery update job processed for node %s", job.NodeID)
}

// GetPendingJobs returns pending jobs from the database (for persistence)
func (q *MasteryQueue) GetPendingJobs(ctx context.Context, db interface{}) ([]*models.MasteryJob, error) {
	// Placeholder - will be implemented with actual database client
	return nil, nil
}

// PersistJob saves a job to the database
func (q *MasteryQueue) PersistJob(ctx context.Context, db interface{}, job *models.MasteryJob) error {
	// Placeholder - will be implemented with actual database client
	return nil
}

// JobID generates a unique job ID
func JobID() string {
	return primitive.NewObjectID().Hex()
}

// JSON conversion helpers
func (j *MasteryUpdateJob) ToJSON() ([]byte, error) {
	return json.Marshal(j)
}

func MasteryUpdateJobFromJSON(data []byte) (*MasteryUpdateJob, error) {
	var job MasteryUpdateJob
	err := json.Unmarshal(data, &job)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal MasteryUpdateJob: %w", err)
	}
	return &job, nil
}

// EnqueueAncestorUpdate creates and enqueues a mastery update job for ancestors
func (q *MasteryQueue) EnqueueAncestorUpdate(nodeID string) error {
	job := &MasteryUpdateJob{
		ID:     JobID(),
		NodeID: nodeID,
	}
	return q.Enqueue(job)
}

// QueueStats returns statistics about the queue
func (q *MasteryQueue) QueueStats() QueueStats {
	return QueueStats{
		Pending:    len(q.jobs),
		Workers:    q.workers,
		Started:    q.started,
		Shutdown:   q.shutdown,
		BufferSize: cap(q.jobs),
	}
}

// QueueStats represents queue statistics
type QueueStats struct {
	Pending    int  `json:"pending"`
	Workers    int  `json:"workers"`
	Started    bool `json:"started"`
	Shutdown   bool `json:"shutdown"`
	BufferSize int  `json:"bufferSize"`
}

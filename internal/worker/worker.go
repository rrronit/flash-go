package worker

import (
	"context"
	"log"
	"sync/atomic"
	"time"

	"flash-go/internal/core"
	"flash-go/internal/isolate"
	"flash-go/internal/redis"
)

const (
	defaultRetries = 3
	queueTimeout   = time.Second
)

// Worker consumes jobs from Redis and executes them.
type Worker struct {
	redis    *redis.Client
	executor *isolate.Executor
}

// New creates a worker with its dependencies.
func New(redisClient *redis.Client) *Worker {
	return &Worker{
		redis:    redisClient,
		executor: isolate.NewExecutor(),
	}
}

// Start launches worker goroutines or an event loop and blocks until they stop.
func (w *Worker) Start(ctx context.Context, concurrency int, poolType string) {
	switch poolType {
	case "thread":
		for i := 0; i < concurrency; i++ {
			go w.runLoop(ctx, i)
		}
		select {}
	case "event":
		w.eventLoop(ctx, concurrency)
	default:
		panic("invalid pool type")
	}
}

func (w *Worker) runLoop(ctx context.Context, idx int) {
	for {
		job, err := w.redis.GetJobFromQueue(ctx, queueTimeout)
		if err != nil {
			log.Printf("worker %d: queue error: %v", idx, err)
			time.Sleep(time.Second)
			continue
		}
		if job == nil {
			continue
		}
		w.processJob(ctx, job, idx)
	}
}

func (w *Worker) eventLoop(ctx context.Context, concurrency int) {
	if concurrency < 1 {
		concurrency = 1
	}
	sem := make(chan struct{}, concurrency)
	var counter int64
	for {
		job, err := w.redis.GetJobFromQueue(ctx, queueTimeout)
		if err != nil {
			log.Printf("event loop: queue error: %v", err)
			time.Sleep(time.Second)
			continue
		}
		if job == nil {
			continue
		}
		sem <- struct{}{}
		workerID := int(atomic.AddInt64(&counter, 1))
		go func(j *core.Job, idx int) {
			defer func() { <-sem }()
			w.processJob(ctx, j, idx)
		}(job, workerID)
	}
}

func (w *Worker) processJob(ctx context.Context, job *core.Job, idx int) {
	for attempt := 0; attempt < defaultRetries; attempt++ {
		job.Status = core.JobStatus{Kind: core.StatusProcessing}
		job.StartedAt = time.Now().UnixNano()
		_ = w.redis.StoreJob(ctx, job)

		_, execErr := w.executor.Execute(ctx, job)
		_ = w.redis.StoreJob(ctx, job)
		w.executor.Cleanup(job.ID)

		if execErr == nil {
			break
		}
		if attempt+1 >= defaultRetries {
			log.Printf("worker %d: job %d failed after %d retries: %v", idx, job.ID, defaultRetries, execErr)
			break
		}
		log.Printf("worker %d: retrying job %d after error: %v", idx, job.ID, execErr)
	}
}

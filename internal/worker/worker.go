package worker

import (
	"context"
	"time"

	"flash-go/internal/isolate"
	"flash-go/internal/models"
	"flash-go/internal/redis"

	"github.com/sirupsen/logrus"
)

const (
	defaultRetries = 3
	queueTimeout   = time.Second
)

type Worker struct {
	redis    *redis.Client
	executor *isolate.Executor
}

func New(redisClient *redis.Client) *Worker {
	return &Worker{
		redis: redisClient,
	}
}

func (w *Worker) Start(ctx context.Context, concurrency int, useBoxPool bool) {
	poolSize := concurrency * 2
	if poolSize < 1 {
		poolSize = 1
	}
	if w.executor == nil {
		w.executor = isolate.NewExecutor(poolSize, useBoxPool)
	}

	for i := 0; i < concurrency; i++ {
		go w.runLoopWithRecover(ctx, i)
	}

	<-ctx.Done()
	logrus.Info("worker shutdown initiated")
}

func (w *Worker) runLoopWithRecover(ctx context.Context, idx int) {
	defer func() {
		if r := recover(); r != nil {
			logrus.WithFields(logrus.Fields{
				"worker_id": idx,
				"panic":     r,
			}).Error("worker panic, respawning")
			go w.runLoopWithRecover(ctx, idx)
		}
	}()
	w.runLoop(ctx, idx)
}

func (w *Worker) runLoop(ctx context.Context, idx int) {
	mainProcessCount := 0
	for {
		select {
		case <-ctx.Done():
			logrus.WithField("worker_id", idx).Info("worker stopping")
			return
		default:
		}

		preferFree := mainProcessCount%3 == 0
		job, err := w.nextJob(ctx, preferFree)
		if err != nil {
			logrus.WithError(err).WithField("worker_id", idx).Error("queue error in worker runLoop")
			time.Sleep(time.Second / 2)
			continue
		}
		mainProcessCount++

		if job == nil {
			continue
		}

		w.processJob(ctx, job, idx)
	}
}

func (w *Worker) nextJob(ctx context.Context, preferFree bool) (*models.Job, error) {
	if preferFree {
		job, err := w.redis.GetJobFromFreeQueue(ctx, queueTimeout)
		if err != nil {
			return nil, err
		}
		if job != nil {
			return job, nil
		}
	}

	job, err := w.redis.GetJobFromMainQueue(ctx, queueTimeout)
	if err != nil {
		return nil, err
	}
	return job, nil
}

func (w *Worker) processJob(ctx context.Context, job *models.Job, idx int) {
	for attempt := 0; attempt < defaultRetries; attempt++ {
		job.Status = models.JobStatus{Kind: models.StatusProcessing}
		job.StartedAt = time.Now().UnixNano()

		if err := w.redis.StoreJob(ctx, job); err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"worker_id": idx,
				"job_id":    job.ID,
				"attempt":   attempt + 1,
			}).Error("failed to store job status in processJob")
		}

		_, execErr := w.executor.Execute(ctx, job)

		if err := w.redis.StoreJob(ctx, job); err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"worker_id": idx,
				"job_id":    job.ID,
				"attempt":   attempt + 1,
			}).Error("failed to store job result in processJob")
		}

		w.executor.Cleanup(job.ID)

		if execErr == nil {
			return
		}

		if attempt+1 >= defaultRetries {
			logrus.WithError(execErr).WithFields(logrus.Fields{
				"worker_id": idx,
				"job_id":    job.ID,
				"retries":   defaultRetries,
			}).Error("job failed after all retries")
			return
		}

		logrus.WithError(execErr).WithFields(logrus.Fields{
			"worker_id": idx,
			"job_id":    job.ID,
			"attempt":   attempt + 1,
		}).Warn("retrying job after error")

		time.Sleep(time.Second) // Brief delay before retry
	}
}

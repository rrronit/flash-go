package redis

import (
	"context"
	"errors"
	"strconv"
	"time"

	"flash-go/internal/models"
	"flash-go/internal/utils"

	redislib "github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

const (
	jobQueueName     = "jobs"
	freeJobQueueName = "free_jobs"
	jobTTL           = time.Hour
)

// Client wraps Redis operations for jobs.
type Client struct {
	rdb *redislib.Client
}

func New(redisURL string) (*Client, error) {
	opts, err := redislib.ParseURL(redisURL)
	if err != nil {
		logrus.WithError(err).WithField("redis_url", redisURL).Error("failed to parse Redis URL")
		return nil, err
	}
	rdb := redislib.NewClient(opts)
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		logrus.WithError(err).WithField("redis_url", redisURL).Error("failed to ping Redis")
		return nil, err
	}
	return &Client{rdb: rdb}, nil
}

func (c *Client) CreateJob(ctx context.Context, job *models.Job) error {
	return c.enqueueJob(ctx, job, jobQueueName)
}

func (c *Client) CreateFreeJob(ctx context.Context, job *models.Job) error {
	return c.enqueueJob(ctx, job, freeJobQueueName)
}

func (c *Client) enqueueJob(ctx context.Context, job *models.Job, queueName string) error {
	payload, err := utils.MarshalJob(job)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"job_id": job.ID,
			"queue":  queueName,
		}).Error("failed to marshal job in enqueueJob")
		return err
	}
	key := utils.JobKey(job.ID)
	pipe := c.rdb.TxPipeline()
	pipe.Set(ctx, key, payload, jobTTL)
	pipe.RPush(ctx, queueName, strconv.FormatUint(job.ID, 10))
	_, err = pipe.Exec(ctx)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"job_id": job.ID,
			"queue":  queueName,
		}).Error("failed to execute Redis pipeline in enqueueJob")
	}
	return err
}

// QueueLength returns the current number of jobs waiting in the queue.
func (c *Client) QueueLength(ctx context.Context, free bool) (int64, error) {
	queueName := jobQueueName

	if free {
		queueName = freeJobQueueName
	}

	length, err := c.rdb.LLen(ctx, queueName).Result()
	if err != nil {
		logrus.WithError(err).Error("failed to get queue length")
	}
	return length, err
}

// StoreJob updates the stored job by ID.
func (c *Client) StoreJob(ctx context.Context, job *models.Job) error {
	payload, err := utils.MarshalJob(job)
	if err != nil {
		logrus.WithError(err).WithField("job_id", job.ID).Error("failed to marshal job in StoreJob")
		return err
	}
	err = c.rdb.Set(ctx, utils.JobKey(job.ID), payload, jobTTL).Err()
	if err != nil {
		logrus.WithError(err).WithField("job_id", job.ID).Error("failed to store job in Redis")
	}
	return err
}

// GetJob fetches a job by ID. Returns (nil, nil) if not found.
func (c *Client) GetJob(ctx context.Context, jobID uint64) (*models.Job, error) {
	data, err := c.rdb.Get(ctx, utils.JobKey(jobID)).Bytes()
	if err != nil {
		if errors.Is(err, redislib.Nil) {
			return nil, nil
		}
		logrus.WithError(err).WithField("job_id", jobID).Error("failed to get job from Redis")
		return nil, err
	}
	var job models.Job
	if err := utils.UnmarshalJob(data, &job); err != nil {
		logrus.WithError(err).WithField("job_id", jobID).Error("failed to unmarshal job")
		return nil, err
	}
	return &job, nil
}

// GetJobFromQueue blocks until a job is available or timeout occurs.
// Uses FIFO (RPush + BLPop) to avoid starving older jobs.
func (c *Client) GetJobFromMainQueue(ctx context.Context, timeout time.Duration) (*models.Job, error) {
	return c.GetJobFromQueue(ctx, timeout, jobQueueName)
}



func (c *Client) GetJobFromFreeQueue(ctx context.Context, timeout time.Duration) (*models.Job, error) {
	return c.GetJobFromQueue(ctx, timeout, freeJobQueueName)
}

// GetJobFromQueue blocks until a job is available or timeout occurs.
// Uses FIFO (RPush + BLPop) to avoid starving older jobs.
func (c *Client) GetJobFromQueue(ctx context.Context, timeout time.Duration, queueName string) (*models.Job, error) {
	result, err := c.rdb.BLPop(ctx, timeout, queueName).Result()
	if err != nil {
		if errors.Is(err, redislib.Nil) {
			return nil, nil
		}
		logrus.WithError(err).WithField("queue", queueName).Error("failed to get job from queue")
		return nil, err
	}
	if len(result) < 2 {
		logrus.WithField("result_length", len(result)).Error("unexpected BLPOP response")
		return nil, errors.New("unexpected BLPOP response in GetJobFromQueue")
	}
	jobID, err := strconv.ParseUint(result[1], 10, 64)
	if err != nil {
		logrus.WithError(err).WithField("job_id_str", result[1]).WithField("queue", queueName).Error("invalid job id in queue")
		return nil, errors.New("invalid job id in queue in GetJobFromQueue")
	}
	return c.GetJob(ctx, jobID)
}

// GetJobs fetches jobs by ID in a single round trip. Missing jobs are nil.
func (c *Client) GetJobs(ctx context.Context, jobIDs []uint64) ([]*models.Job, error) {
	if len(jobIDs) == 0 {
		return nil, nil
	}
	keys := make([]string, 0, len(jobIDs))
	for _, jobID := range jobIDs {
		keys = append(keys, utils.JobKey(jobID))
	}
	values, err := c.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		logrus.WithError(err).WithField("job_count", len(jobIDs)).Error("failed to get jobs from Redis")
		return nil, err
	}
	jobs := make([]*models.Job, len(jobIDs))
	for i, value := range values {
		if value == nil {
			continue
		}
		var data []byte
		switch v := value.(type) {
		case string:
			data = []byte(v)
		case []byte:
			data = v
		default:
			logrus.WithField("value_type", v).WithField("job_index", i).Error("unexpected redis value type")
			return nil, errors.New("unexpected redis value type")
		}
		var job models.Job
		if err := utils.UnmarshalJob(data, &job); err != nil {
			logrus.WithError(err).WithField("job_index", i).Error("failed to unmarshal job in GetJobs")
			return nil, err
		}
		jobs[i] = &job
	}
	return jobs, nil
}

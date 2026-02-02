package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	"flash-go/internal/core"

	jsoniter "github.com/json-iterator/go"
	redislib "github.com/redis/go-redis/v9"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

const jobQueueName = "jobs"

// Client wraps Redis operations for jobs.
type Client struct {
	rdb *redislib.Client
}

// New creates a new Redis client using a URL.
func New(redisURL string) (*Client, error) {
	opts, err := redislib.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	rdb := redislib.NewClient(opts)
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		return nil, err
	}
	return &Client{rdb: rdb}, nil
}

// CreateJob stores a job and enqueues it atomically.
func (c *Client) CreateJob(ctx context.Context, job *core.Job) error {
	payload, err := json.Marshal(job)
	if err != nil {
		return err
	}
	key := jobKey(job.ID)
	pipe := c.rdb.TxPipeline()
	pipe.Set(ctx, key, payload, 0)
	pipe.RPush(ctx, jobQueueName, payload)
	_, err = pipe.Exec(ctx)
	return err
}

// StoreJob updates the stored job by ID.
func (c *Client) StoreJob(ctx context.Context, job *core.Job) error {
	payload, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, jobKey(job.ID), payload, 0).Err()
}

// GetJob fetches a job by ID. Returns (nil, nil) if not found.
func (c *Client) GetJob(ctx context.Context, jobID uint64) (*core.Job, error) {
	data, err := c.rdb.Get(ctx, jobKey(jobID)).Bytes()
	if err != nil {
		if errors.Is(err, redislib.Nil) {
			return nil, nil
		}
		return nil, err
	}
	var job core.Job
	if err := json.Unmarshal(data, &job); err != nil {
		return nil, err
	}
	return &job, nil
}

// GetJobFromQueue blocks until a job is available or timeout occurs.
func (c *Client) GetJobFromQueue(ctx context.Context, timeout time.Duration) (*core.Job, error) {
	result, err := c.rdb.BRPop(ctx, timeout, jobQueueName).Result()
	if err != nil {
		if errors.Is(err, redislib.Nil) {
			return nil, nil
		}
		return nil, err
	}
	if len(result) < 2 {
		return nil, errors.New("unexpected BRPOP response")
	}
	var job core.Job
	if err := json.Unmarshal([]byte(result[1]), &job); err != nil {
		return nil, err
	}
	return &job, nil
}

func jobKey(id uint64) string {
	return fmt.Sprintf("%d", id)
}

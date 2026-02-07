package api

import (
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"

	"flash-go/internal/core"
	"flash-go/internal/models"
	"flash-go/internal/redis"
	"flash-go/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type Handler struct {
	redis             *redis.Client
	queueLengthLimit  int64
	workerConcurrency int
	useBoxPool        bool
}

type preparedSubmission struct {
	sourceCode     string
	stdin          string
	expectedOutput string
	lang           models.Language
	settings       models.ExecutionSettings
}

func NewHandler(redisClient *redis.Client, queueLengthLimit int, workerConcurrency int, useBoxPool bool) *Handler {
	return &Handler{
		redis:             redisClient,
		queueLengthLimit:  int64(queueLengthLimit),
		workerConcurrency: workerConcurrency,
		useBoxPool:        useBoxPool,
	}
}

func RegisterRoutes(router *gin.Engine, handler *Handler) {
	router.POST("/create", handler.Create)
	router.GET("/check/:job_id", handler.Check)
	router.GET("/health", handler.Health)
	router.POST("/submissions/batch", handler.SubmitBatch)
	router.GET("/submissions/batch", handler.GetBatch)
}

func (h *Handler) hasQueueCapacity(ctx *gin.Context, free bool, incoming int) (bool, error) {
	if h.queueLengthLimit <= 0 {
		return true, nil
	}
	length, err := h.redis.QueueLength(ctx.Request.Context(), free)
	if err != nil {
		logrus.WithError(err).Error("failed to check queue length")
		return false, err
	}
	return length+int64(incoming) <= h.queueLengthLimit, nil
}

// Create enqueues a new job.
func (h *Handler) Create(c *gin.Context) {
	var req models.CreateJobRequest
	if err := utils.BindJSONFast(c, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if ok, err := h.hasQueueCapacity(c, req.Free, 1); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check queue length"})
		return
	} else if !ok {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "queue limit reached"})
		return
	}

	lang, ok := core.LanguageFor(req.Language)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported language"})
		return
	}

	settings := core.DefaultExecutionSettings()
	if req.TimeLimit != nil {
		settings.CPUTimeLimit = *req.TimeLimit
	}
	if req.MemoryLimit != nil {
		settings.MemoryLimit = *req.MemoryLimit
	}
	if req.StackLimit != nil {
		settings.StackLimit = *req.StackLimit
	}

	job := core.NewJob(req.Code, req.Input, req.Expected, lang, settings)

	var err error
	if req.Free {
		err = h.redis.CreateFreeJob(c.Request.Context(), &job)
	} else {
		err = h.redis.CreateJob(c.Request.Context(), &job)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enqueue job"})
		return
	}

	c.JSON(http.StatusOK, models.CreateJobResponse{
		Status: "created",
		ID:     strconv.FormatUint(job.ID, 10),
	})
}

// Check returns a job status by ID.
func (h *Handler) Check(c *gin.Context) {
	idStr := c.Param("job_id")
	jobID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		logrus.WithError(err).WithField("job_id_str", idStr).Error("invalid job id in Check")
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job id"})
		return
	}

	job, err := h.redis.GetJob(c.Request.Context(), jobID)
	if err != nil {
		logrus.WithError(err).WithField("job_id", jobID).Error("failed to fetch job in Check")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch job"})
		return
	}
	if job == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	c.JSON(http.StatusOK, models.CheckResponse{
		CreatedAt:     job.CreatedAt,
		StartedAt:     job.StartedAt,
		FinishedAt:    job.FinishedAt,
		Stdout:        job.Output.Stdout,
		Time:          job.Output.Time,
		Memory:        job.Output.Memory,
		Stderr:        job.Output.Stderr,
		Token:         job.ID,
		CompileOutput: job.Output.CompileOutput,
		Message:       job.Output.Message,
		Status: models.CheckStatus{
			ID:          job.Status.ID(),
			Description: job.Status.Description(),
		},
	})
}

// Health returns a simple health response.
func (h *Handler) Health(c *gin.Context) {
	queueLength, err := h.redis.QueueLength(c.Request.Context(), false)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error", "error": "failed to check queue length"})
		return
	}
	freeQueueLength, err := h.redis.QueueLength(c.Request.Context(), true)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error", "error": "failed to check free queue length"})
		return
	}

	response := gin.H{
		"status":             "ok",
		"queued_jobs":        queueLength,
		"queue_limit":        h.queueLengthLimit,
		"worker_concurrency": h.workerConcurrency,
		"box_pool":           h.useBoxPool,
		"free_queued_jobs":   freeQueueLength,
		"free_queue_limit":   h.queueLengthLimit,
	}
	if h.queueLengthLimit > 0 {
		response["queue_available"] = h.queueLengthLimit - queueLength
		response["queue_utilization"] = float64(queueLength) / float64(h.queueLengthLimit)
	}

	c.JSON(http.StatusOK, response)
}

// SubmitBatch handles POST /submissions/batch?base64_encoded=true
// Accepts a batch of submissions and returns tokens for each.
func (h *Handler) SubmitBatch(c *gin.Context) {
	base64Encoded := c.Query("base64_encoded") == "true"

	var req models.Judge0BatchSubmissionRequest
	if err := utils.BindJSONFast(c, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if len(req.Submissions) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "submissions array cannot be empty"})
		return
	}

	if ok, err := h.hasQueueCapacity(c,req.Free, len(req.Submissions)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check queue length"})
		return
	} else if !ok {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "queue limit reached"})
		return
	}
	

	prepared := make([]preparedSubmission, 0, len(req.Submissions))

	for _, sub := range req.Submissions {
		sourceCode := sub.SourceCode
		stdin := sub.Stdin
		expectedOutput := sub.ExpectedOutput

		if base64Encoded {
			decoded, err := base64.StdEncoding.DecodeString(sourceCode)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid base64 source_code"})
				return
			}
			sourceCode = string(decoded)

			if stdin != "" {
				decoded, err := base64.StdEncoding.DecodeString(stdin)
				if err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid base64 stdin"})
					return
				}
				stdin = string(decoded)
			}

			if expectedOutput != "" {
				decoded, err := base64.StdEncoding.DecodeString(expectedOutput)
				if err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid base64 expected_output"})
					return
				}
				expectedOutput = string(decoded)
			}
		}

		langName, ok := utils.Judge0LanguageIDToName(sub.LanguageID)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported language_id"})
			return
		}

		lang, ok := core.LanguageFor(langName)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported language"})
			return
		}

		settings := core.DefaultExecutionSettings()
		if sub.CPUTimeLimit > 0 {
			settings.CPUTimeLimit = sub.CPUTimeLimit
		}
		if sub.MemoryLimit > 0 {
			settings.MemoryLimit = uint64(sub.MemoryLimit)
		}
		if sub.MaxProcessesAndOrThreads > 0 {
			settings.MaxProcesses = uint32(sub.MaxProcessesAndOrThreads)
		}

		prepared = append(prepared, preparedSubmission{
			sourceCode:     sourceCode,
			stdin:          stdin,
			expectedOutput: expectedOutput,
			lang:           lang,
			settings:       settings,
		})
	}

	responses := make([]models.Judge0SubmissionResponse, 0, len(prepared))
	for _, sub := range prepared {
		job := core.NewJob(sub.sourceCode, sub.stdin, sub.expectedOutput, sub.lang, sub.settings)
		var err error
		if req.Free {
			err = h.redis.CreateFreeJob(c.Request.Context(), &job)
		} else {
			err = h.redis.CreateJob(c.Request.Context(), &job)
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enqueue job"})
			return
		}

		responses = append(responses, models.Judge0SubmissionResponse{
			Token: strconv.FormatUint(job.ID, 10),
		})
	}

	c.JSON(http.StatusCreated, responses)
}

// GetBatch handles GET /submissions/batch?tokens={tokens}&base64_encoded=false
// Retrieves the status and results of batch submissions by tokens.
func (h *Handler) GetBatch(c *gin.Context) {
	tokensStr := c.Query("tokens")
	if tokensStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tokens parameter is required"})
		return
	}

	tokenStrs := strings.Split(tokensStr, ",")
	if len(tokenStrs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one token is required"})
		return
	}

	jobIDs := make([]uint64, 0, len(tokenStrs))
	for _, tokenStr := range tokenStrs {
		tokenStr = strings.TrimSpace(tokenStr)
		if tokenStr == "" {
			continue
		}
		jobID, err := strconv.ParseUint(tokenStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid token format"})
			return
		}
		jobIDs = append(jobIDs, jobID)
	}

	if len(jobIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no valid tokens provided"})
		return
	}

	// Always use batch fetch - if it fails, return error instead of N+1 queries
	jobs, err := h.redis.GetJobs(c.Request.Context(), jobIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch jobs"})
		return
	}

	submissions := make([]models.Judge0SubmissionDetails, 0, len(jobIDs))
	for i, jobID := range jobIDs {
		var job *models.Job
		if i < len(jobs) {
			job = jobs[i]
		}
		if job == nil {
			submissions = append(submissions, models.Judge0SubmissionDetails{
				Token: strconv.FormatUint(jobID, 10),
				Status: models.Judge0Status{
					ID:          13,
					Description: "Internal Error",
				},
			})
			continue
		}

		details := models.Judge0SubmissionDetails{
			Token: strconv.FormatUint(job.ID, 10),
			Status: models.Judge0Status{
				ID:          job.Status.ID(),
				Description: job.Status.Description(),
			},
			CreatedAt:  job.CreatedAt,
			StartedAt:  job.StartedAt,
			FinishedAt: job.FinishedAt,
		}

		if job.Output.Stdout != "" {
			details.Stdout = &job.Output.Stdout
		}
		if job.Output.Stderr != "" {
			details.Stderr = &job.Output.Stderr
		}
		if job.Output.CompileOutput != "" {
			details.CompileOutput = &job.Output.CompileOutput
		}
		if job.Output.Message != "" {
			details.Message = &job.Output.Message
		}
		if job.Output.Time > 0 {
			timeStr := strconv.FormatFloat(job.Output.Time, 'f', -1, 64)
			details.Time = &timeStr
		}
		if job.Output.Memory > 0 {
			memory := int(job.Output.Memory)
			details.Memory = &memory
		}

		submissions = append(submissions, details)
	}

	c.JSON(http.StatusOK, models.Judge0BatchResponse{
		Submissions: submissions,
	})
}

package api

import (
	"net/http"
	"strconv"

	"flash-go/internal/core"
	"flash-go/internal/redis"

	"github.com/gin-gonic/gin"
)

// Handler wires HTTP routes to Redis-backed logic.
type Handler struct {
	redis *redis.Client
}

// NewHandler creates a new API handler.
func NewHandler(redisClient *redis.Client) *Handler {
	return &Handler{redis: redisClient}
}

// RegisterRoutes registers all API routes.
func RegisterRoutes(router *gin.Engine, handler *Handler) {
	router.POST("/create", handler.Create)
	router.GET("/check/:job_id", handler.Check)
	router.GET("/health", handler.Health)
}

type createJobRequest struct {
	Code        string   `json:"code" binding:"required"`
	Language    string   `json:"language" binding:"required"`
	Input       string   `json:"input"`
	Expected    string   `json:"expected"`
	TimeLimit   *float64 `json:"time_limit"`
	MemoryLimit *uint64  `json:"memory_limit"`
	StackLimit  *uint64  `json:"stack_limit"`
}

type createJobResponse struct {
	Status string `json:"status"`
	ID     string `json:"id"`
}

// Create enqueues a new job.
func (h *Handler) Create(c *gin.Context) {
	var req createJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
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
	if err := h.redis.CreateJob(c.Request.Context(), &job); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enqueue job"})
		return
	}

	c.JSON(http.StatusOK, createJobResponse{
		Status: "created",
		ID:     strconv.FormatUint(job.ID, 10),
	})
}

type checkStatus struct {
	ID          int    `json:"id"`
	Description string `json:"description"`
}

type checkResponse struct {
	CreatedAt     int64       `json:"created_at"`
	StartedAt     int64       `json:"started_at"`
	FinishedAt    int64       `json:"finished_at"`
	Stdout        string      `json:"stdout"`
	Time          float64     `json:"time"`
	Memory        uint64      `json:"memory"`
	Stderr        string      `json:"stderr"`
	Token         uint64      `json:"token"`
	CompileOutput string      `json:"compile_output"`
	Message       string      `json:"message"`
	Status        checkStatus `json:"status"`
}

// Check returns a job status by ID.
func (h *Handler) Check(c *gin.Context) {
	idStr := c.Param("job_id")
	jobID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job id"})
		return
	}

	job, err := h.redis.GetJob(c.Request.Context(), jobID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch job"})
		return
	}
	if job == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	c.JSON(http.StatusOK, checkResponse{
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
		Status: checkStatus{
			ID:          job.Status.ID(),
			Description: job.Status.Description(),
		},
	})
}

// Health returns a simple health response.
func (h *Handler) Health(c *gin.Context) {
	c.String(http.StatusOK, "ok")
}

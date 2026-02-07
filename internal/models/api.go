package models


// CreateJobRequest represents the request body for creating a new job.
type CreateJobRequest struct {
	Code        string   `json:"code"`
	Input       string   `json:"input"`
	Expected    string   `json:"expected"`
	Language    string   `json:"language"`
	TimeLimit   *float64 `json:"time_limit,omitempty"`
	MemoryLimit *uint64  `json:"memory_limit,omitempty"`
	StackLimit  *uint64  `json:"stack_limit,omitempty"`
	Free        bool     `json:"free"`
}

// CreateJobResponse represents the response after creating a job.
type CreateJobResponse struct {
	Status string `json:"status"`
	ID     string `json:"id"`
}

// CheckStatus represents the status information in a check response.
type CheckStatus struct {
	ID          int    `json:"id"`
	Description string `json:"description"`
}

// CheckResponse represents the response when checking a job status.
type CheckResponse struct {
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
	Status        CheckStatus `json:"status"`
}

// Judge0Status represents a Judge0-compatible status.
type Judge0Status struct {
	ID          int    `json:"id"`
	Description string `json:"description"`
}

// Judge0Submission represents a single submission in a batch request.
type Judge0Submission struct {
	SourceCode               string  `json:"source_code"`
	LanguageID               int     `json:"language_id"`
	Stdin                    string  `json:"stdin,omitempty"`
	ExpectedOutput           string  `json:"expected_output,omitempty"`
	CPUTimeLimit             float64 `json:"cpu_time_limit,omitempty"`
	MemoryLimit              int     `json:"memory_limit,omitempty"`
	MaxProcessesAndOrThreads int     `json:"max_processes_and_or_threads,omitempty"`
}

// Judge0BatchSubmissionRequest represents a batch submission request.
type Judge0BatchSubmissionRequest struct {
	Submissions []Judge0Submission `json:"submissions"`
	Free        bool               `json:"free"`
}

// Judge0SubmissionResponse represents the response for a single submission.
type Judge0SubmissionResponse struct {
	Token string `json:"token"`
}

// Judge0SubmissionDetails represents detailed information about a submission.
type Judge0SubmissionDetails struct {
	Token         string       `json:"token"`
	Status        Judge0Status `json:"status"`
	CreatedAt     int64        `json:"created_at"`
	StartedAt     int64        `json:"started_at,omitempty"`
	FinishedAt    int64        `json:"finished_at,omitempty"`
	Stdout        *string      `json:"stdout,omitempty"`
	Stderr        *string      `json:"stderr,omitempty"`
	CompileOutput *string      `json:"compile_output,omitempty"`
	Message       *string      `json:"message,omitempty"`
	Time          *string      `json:"time,omitempty"`
	Memory        *int         `json:"memory,omitempty"`
}

// Judge0BatchResponse represents the response for a batch query.
type Judge0BatchResponse struct {
	Submissions []Judge0SubmissionDetails `json:"submissions"`
}

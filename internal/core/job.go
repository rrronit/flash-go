package core

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"
)

const (
	StatusQueued            = "Queued"
	StatusProcessing        = "Processing"
	StatusAccepted          = "Accepted"
	StatusWrongAnswer       = "WrongAnswer"
	StatusTimeLimitExceeded = "TimeLimitExceeded"
	StatusCompilationError  = "CompilationError"
	StatusRuntimeError      = "RuntimeError"
	StatusInternalError     = "InternalError"
	StatusExecFormatError   = "ExecFormatError"
)

// JobStatus represents the current state of a job.
type JobStatus struct {
	Kind        string `json:"kind"`
	RuntimeCode string `json:"runtime_code,omitempty"`
}

// ID returns the Judge0-style status ID used by the API.
func (s JobStatus) ID() int {
	switch s.Kind {
	case StatusQueued:
		return 1
	case StatusProcessing:
		return 2
	case StatusAccepted:
		return 3
	case StatusWrongAnswer:
		return 4
	case StatusTimeLimitExceeded:
		return 5
	case StatusCompilationError:
		return 6
	case StatusRuntimeError:
		switch s.RuntimeCode {
		case "SIGSEGV":
			return 7
		case "SIGXFSZ":
			return 8
		case "SIGFPE":
			return 9
		case "SIGABRT":
			return 10
		case "NZEC":
			return 11
		default:
			return 12
		}
	case StatusInternalError:
		return 13
	case StatusExecFormatError:
		return 14
	default:
		return 13
	}
}

// Description returns the human-readable status string used by the API.
func (s JobStatus) Description() string {
	switch s.Kind {
	case StatusQueued:
		return "In Queue"
	case StatusProcessing:
		return "Processing"
	case StatusAccepted:
		return "Accepted"
	case StatusWrongAnswer:
		return "Wrong Answer"
	case StatusTimeLimitExceeded:
		return "Time Limit Exceeded"
	case StatusCompilationError:
		return "Compilation Error"
	case StatusRuntimeError:
		if s.RuntimeCode == "" {
			return "Runtime Error"
		}
		return fmt.Sprintf("Runtime Error: (%s)", s.RuntimeCode)
	case StatusInternalError:
		return "Internal Error"
	case StatusExecFormatError:
		return "Exec Format Error"
	default:
		return "Internal Error"
	}
}

// JobOutput captures program output and execution metadata.
type JobOutput struct {
	Stdout        string  `json:"stdout"`
	Stderr        string  `json:"stderr"`
	CompileOutput string  `json:"compile_output"`
	Time          float64 `json:"time"`
	Memory        uint64  `json:"memory"`
	ExitCode      int     `json:"exit_code"`
	Message       string  `json:"message"`
}

// Job represents a unit of work in the judge.
type Job struct {
	ID             uint64            `json:"id"`
	SourceCode     string            `json:"source_code"`
	Language       Language          `json:"language"`
	Stdin          string            `json:"stdin"`
	ExpectedOutput string            `json:"expected_output"`
	Settings       ExecutionSettings `json:"settings"`
	Status         JobStatus         `json:"status"`
	CreatedAt      int64             `json:"created_at"`
	StartedAt      int64             `json:"started_at"`
	FinishedAt     int64             `json:"finished_at"`
	Output         JobOutput         `json:"output"`
}

// NewJob constructs a new job with defaults.
func NewJob(sourceCode, stdin, expected string, language Language, settings ExecutionSettings) Job {
	return Job{
		ID:             NewJobID(),
		SourceCode:     sourceCode,
		Language:       language,
		Stdin:          stdin,
		ExpectedOutput: expected,
		Settings:       settings,
		Status:         JobStatus{Kind: StatusQueued},
		CreatedAt:      time.Now().UnixNano(),
		Output:         JobOutput{},
	}
}

// NewJobID generates a random job ID.
func NewJobID() uint64 {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err == nil {
		id := binary.LittleEndian.Uint64(buf[:])
		if id != 0 {
			return id
		}
	}
	return uint64(time.Now().UnixNano())
}

// RuntimeErrorStatus creates a runtime error status.
func RuntimeErrorStatus(code string) JobStatus {
	return JobStatus{
		Kind:        StatusRuntimeError,
		RuntimeCode: code,
	}
}

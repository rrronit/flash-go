package models

import "fmt"

// Status constants for job processing.
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

// Language describes how to compile and run a job.
type Language struct {
	Name       string `json:"name"`
	SourceFile string `json:"source_file"`
	CompileCmd string `json:"compile_cmd"`
	RunCmd     string `json:"run_cmd"`
	IsCompiled bool   `json:"is_compiled"`
}

// ExecutionSettings defines resource limits for a job.
type ExecutionSettings struct {
	CPUTimeLimit  float64 `json:"cpu_time_limit"`
	WallTimeLimit float64 `json:"wall_time_limit"`
	MemoryLimit   uint64  `json:"memory_limit"`
	StackLimit    uint64  `json:"stack_limit"`
	MaxProcesses  uint32  `json:"max_processes"`
	MaxFileSize   uint64  `json:"max_file_size"`
	EnableNetwork bool    `json:"enable_network"`
	EnablePerProcessAndThreadTimeLimit    bool    `json:"enable_per_process_and_thread_time_limit,omitempty"`
	EnablePerProcessAndThreadMemoryLimit  bool    `json:"enable_per_process_and_thread_memory_limit,omitempty"`
	RedirectStderrToStdout                bool    `json:"redirect_stderr_to_stdout,omitempty"`
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

// JobPaths holds file paths for a job execution sandbox.
type JobPaths struct {
	BoxPath           string
	MetadataPath      string
	StdoutPath        string
	StderrPath        string
	StdinPath         string
	CompileOutputPath string
}

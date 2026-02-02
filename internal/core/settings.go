package core

// ExecutionSettings defines resource limits for a job.
type ExecutionSettings struct {
	CPUTimeLimit  float64 `json:"cpu_time_limit"`
	WallTimeLimit float64 `json:"wall_time_limit"`
	MemoryLimit   uint64  `json:"memory_limit"`
	StackLimit    uint64  `json:"stack_limit"`
	MaxProcesses  uint32  `json:"max_processes"`
	MaxFileSize   uint64  `json:"max_file_size"`
	EnableNetwork bool    `json:"enable_network"`
}

// DefaultExecutionSettings returns the default limits used by the server.
func DefaultExecutionSettings() ExecutionSettings {
	return ExecutionSettings{
		CPUTimeLimit:  2.0,
		WallTimeLimit: 5.0,
		MemoryLimit:   128_000,
		StackLimit:    64_000,
		MaxProcesses:  60,
		MaxFileSize:   4096,
		EnableNetwork: false,
	}
}


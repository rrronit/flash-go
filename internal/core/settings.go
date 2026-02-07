package core

import "flash-go/internal/models"

// DefaultExecutionSettings returns the default resource limits used by the server.
func DefaultExecutionSettings() models.ExecutionSettings {
	return models.ExecutionSettings{
		CPUTimeLimit:  2.0,
		WallTimeLimit: 5.0,
		MemoryLimit:   128_000,
		StackLimit:    64_000,
		MaxProcesses:  60,
		MaxFileSize:   4096,
		EnableNetwork: false,
	}
}

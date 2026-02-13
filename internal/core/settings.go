package core

import "flash-go/internal/models"

// DefaultExecutionSettings returns the default resource limits used by the server.
func DefaultExecutionSettings() models.ExecutionSettings {
	return models.ExecutionSettings{
		MaxCPUTimeLimit:                      15.0,
		CPUTimeLimit:                         5.0,
		WallTimeLimit:                        10.0,
		MaxWallTimeLimit:                     20.0,
		MemoryLimit:                          128_000,
		MaxMemoryLimit:                       2048_000,
		MaxStackLimit:                        512_000,
		StackLimit:                           64_000,
		MaxProcesses:                         60,
		MaxFileSize:                          1024,
		EnableNetwork:                        false,
		EnablePerProcessAndThreadTimeLimit:   false,
		EnablePerProcessAndThreadMemoryLimit: false,
		RedirectStderrToStdout:               false,
	}
}

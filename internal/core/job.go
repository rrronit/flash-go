package core

import (
	"crypto/rand"
	"encoding/binary"
	"time"

	"flash-go/internal/models"
)

// NewJob constructs a new job with defaults.
func NewJob(sourceCode, stdin, expected string, language models.Language, settings models.ExecutionSettings) models.Job {
	return models.Job{
		ID:             NewJobID(),
		SourceCode:     sourceCode,
		Language:       language,
		Stdin:          stdin,
		ExpectedOutput: expected,
		Settings:       settings,
		Status:         models.JobStatus{Kind: models.StatusQueued},
		CreatedAt:      time.Now().UnixNano(),
		Output:         models.JobOutput{},
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
func RuntimeErrorStatus(code string) models.JobStatus {
	return models.JobStatus{
		Kind:        models.StatusRuntimeError,
		RuntimeCode: code,
	}
}

package utils

import (
	"bufio"
	"io"
	"os"
	"strconv"
	"strings"

	"flash-go/internal/models"
)

// Metadata holds parsed isolate execution metadata.
type Metadata struct {
	Time     float64
	Memory   uint64
	ExitCode int
	Message  string
	Status   string
}

// JobKey returns the Redis key for a job ID.
func JobKey(id uint64) string {
	return "job:" + strconv.FormatUint(id, 10)
}

// ReadFileIfExists reads a file and returns its content as a string.
// Returns an empty string if the file does not exist or cannot be read.
func ReadFileIfExists(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	buf := GetBuffer()
	defer PutBuffer(buf)

	_, err = io.Copy(buf, file)
	if err != nil {
		return ""
	}
	return buf.String()
}

// ReadMetadata parses an isolate metadata file into a Metadata struct.
func ReadMetadata(path string) (Metadata, error) {
	file, err := os.Open(path)
	if err != nil {
		return Metadata{}, err
	}
	defer file.Close()

	var m Metadata
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		
		// Use strings.Cut (Go 1.18+) for efficient splitting
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}

		switch key {
		case "time":
			m.Time, _ = strconv.ParseFloat(value, 64)
		case "max-rss":
			m.Memory, _ = strconv.ParseUint(value, 10, 64)
		case "cg-mem":
			mem, _ := strconv.ParseUint(value, 10, 64)
			if mem > m.Memory {
				m.Memory = mem
			}
		case "exitcode":
			m.ExitCode, _ = strconv.Atoi(value)
		case "message":
			m.Message = value
		case "status":
			m.Status = value
		}
	}

	if err := scanner.Err(); err != nil {
		return Metadata{}, err
	}

	return m, nil
}

// DetermineStatus maps isolate metadata status to a JobStatus.
func DetermineStatus(status string, exitCode int, stdout, expected string) models.JobStatus {
	switch status {
	case "TO":
		return models.JobStatus{Kind: models.StatusTimeLimitExceeded}
	case "SG":
		return findRuntimeType(exitCode)
	case "RE":
		return models.JobStatus{Kind: models.StatusRuntimeError, RuntimeCode: "NZEC"}
	case "XX":
		return models.JobStatus{Kind: models.StatusInternalError}
	default:
		if expected == "" || strings.TrimSpace(stdout) == strings.TrimSpace(expected) {
			return models.JobStatus{Kind: models.StatusAccepted}
		}
		return models.JobStatus{Kind: models.StatusWrongAnswer}
	}
}

// findRuntimeType maps a signal exit code to the appropriate runtime error status.
func findRuntimeType(exitCode int) models.JobStatus {
	switch exitCode {
	case 11:
		return models.JobStatus{Kind: models.StatusRuntimeError, RuntimeCode: "SIGSEGV"}
	case 25:
		return models.JobStatus{Kind: models.StatusRuntimeError, RuntimeCode: "SIGXFSZ"}
	case 8:
		return models.JobStatus{Kind: models.StatusRuntimeError, RuntimeCode: "SIGFPE"}
	case 6:
		return models.JobStatus{Kind: models.StatusRuntimeError, RuntimeCode: "SIGABRT"}
	default:
		return models.JobStatus{Kind: models.StatusRuntimeError, RuntimeCode: "Other"}
	}
}

func DetectCgroupSupport() bool {
    _, err := os.Stat("/sys/fs/cgroup")
    return err == nil
}
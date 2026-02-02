package isolate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"flash-go/internal/core"
)

const (
	isolatePath   = "isolate"
	boxModulo     = 2147483647
	fakeExecution = false
)

// Executor runs jobs inside isolate.
type Executor struct{}

// NewExecutor creates an isolate executor.
func NewExecutor() *Executor {
	return &Executor{}
}

// Execute runs a job inside isolate and updates its fields.
func (e *Executor) Execute(ctx context.Context, job *core.Job) (core.JobStatus, error) {
	if fakeExecution {
		// Fake accept for now to test the executor.
		job.Status = core.JobStatus{Kind: core.StatusAccepted}
		start := time.Now()
		job.StartedAt = start.UnixNano()
		time.Sleep(1 * time.Second)
		job.FinishedAt = time.Now().UnixNano()
		job.Output.Time = time.Since(start).Seconds()
		return job.Status, nil
	}
	boxID := job.ID % boxModulo
	boxPath, err := initBox(ctx, boxID)
	if err != nil {
		job.Status = core.JobStatus{Kind: core.StatusInternalError}
		job.Output.Message = err.Error()
		job.FinishedAt = time.Now().UnixNano()
		return job.Status, err
	}

	paths, err := setupFiles(job, boxPath)
	if err != nil {
		job.Status = core.JobStatus{Kind: core.StatusInternalError}
		job.Output.Message = err.Error()
		job.FinishedAt = time.Now().UnixNano()
		return job.Status, err
	}

	if job.Language.CompileCmd != "" {
		compileStatus, compileErr := compileJob(ctx, job, boxID, paths)
		if compileErr != nil {
			job.Status = core.JobStatus{Kind: core.StatusInternalError}
			job.Output.Message = compileErr.Error()
			job.FinishedAt = time.Now().UnixNano()
			return job.Status, compileErr
		}
		if compileStatus.Kind == core.StatusCompilationError {
			job.Status = compileStatus
			job.FinishedAt = time.Now().UnixNano()
			return job.Status, nil
		}
	}

	runErr := runJob(ctx, job, boxID, paths)
	if runErr != nil && !errors.Is(runErr, context.DeadlineExceeded) {
		job.Status = core.JobStatus{Kind: core.StatusInternalError}
		job.Output.Message = runErr.Error()
		job.FinishedAt = time.Now().UnixNano()
		return job.Status, runErr
	}

	if err := readOutputs(job, paths); err != nil {
		job.Status = core.JobStatus{Kind: core.StatusInternalError}
		job.Output.Message = err.Error()
		job.FinishedAt = time.Now().UnixNano()
		return job.Status, err
	}

	meta, err := readMetadata(paths.MetadataPath)
	if err != nil {
		job.Status = core.JobStatus{Kind: core.StatusInternalError}
		job.Output.Message = err.Error()
		job.FinishedAt = time.Now().UnixNano()
		return job.Status, err
	}

	job.Output.Time = meta.Time
	job.Output.Memory = meta.Memory
	job.Output.ExitCode = meta.ExitCode
	job.Output.Message = meta.Message

	job.Status = determineStatus(meta.Status, meta.ExitCode, job.Output.Stdout, job.ExpectedOutput)
	job.FinishedAt = time.Now().UnixNano()

	return job.Status, nil
}

// Cleanup removes the isolate box for a job.
func (e *Executor) Cleanup(jobID uint64) {
	boxID := jobID % boxModulo
	_ = exec.Command(isolatePath, "--cg", "-b", strconv.FormatUint(boxID, 10), "--cleanup").Run()
}

type jobPaths struct {
	BoxPath           string
	MetadataPath      string
	StdoutPath        string
	StderrPath        string
	StdinPath         string
	CompileOutputPath string
}

func initBox(ctx context.Context, boxID uint64) (string, error) {
	cmd := exec.CommandContext(ctx, isolatePath, "-b", strconv.FormatUint(boxID, 10), "--cg", "--init")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("isolate init failed: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	boxPath := strings.TrimSpace(string(output))
	if boxPath == "" {
		return "", errors.New("isolate init returned empty box path")
	}
	return boxPath, nil
}

func setupFiles(job *core.Job, boxPath string) (jobPaths, error) {
	boxDir := filepath.Join(boxPath, "box")
	sourcePath := filepath.Join(boxDir, job.Language.SourceFile)
	stdinPath := filepath.Join(boxDir, "stdin")
	stdoutPath := filepath.Join(boxDir, "stdout")
	stderrPath := filepath.Join(boxDir, "stderr")
	metadataPath := filepath.Join(boxDir, "metadata")
	compileOutputPath := filepath.Join(boxDir, "compile_output")

	if err := os.WriteFile(sourcePath, []byte(job.SourceCode), 0o644); err != nil {
		return jobPaths{}, fmt.Errorf("write source: %w", err)
	}
	if err := os.WriteFile(stdinPath, []byte(job.Stdin), 0o644); err != nil {
		return jobPaths{}, fmt.Errorf("write stdin: %w", err)
	}

	return jobPaths{
		BoxPath:           boxPath,
		MetadataPath:      metadataPath,
		StdoutPath:        stdoutPath,
		StderrPath:        stderrPath,
		StdinPath:         stdinPath,
		CompileOutputPath: compileOutputPath,
	}, nil
}

func compileJob(ctx context.Context, job *core.Job, boxID uint64, paths jobPaths) (core.JobStatus, error) {
	parts := strings.Fields(job.Language.CompileCmd)
	if len(parts) == 0 {
		return core.JobStatus{Kind: core.StatusInternalError}, errors.New("compile command is empty")
	}
	compileExe := parts[0]
	compileArgs := strings.Join(parts[1:], " ")
	cmdStr := fmt.Sprintf("%s %s 2> /box/compile_output", compileExe, compileArgs)

	args := []string{
		"--cg",
		"-b", strconv.FormatUint(boxID, 10),
		"-M", paths.MetadataPath,
		fmt.Sprintf("--process=%d", job.Settings.MaxProcesses),
		"-t", "5",
		"-x", "0",
		"-w", "10",
		"-k", fmt.Sprintf("%d", job.Settings.StackLimit),
		"-f", fmt.Sprintf("%d", job.Settings.MaxFileSize),
		fmt.Sprintf("--cg-mem=%d", job.Settings.MemoryLimit),
		"-E", "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"-E", "HOME=/tmp",
		"-d", "/etc:noexec",
		"--run",
		"--",
		"/usr/bin/sh",
		"-c",
		cmdStr,
	}

	output, err := exec.CommandContext(ctx, isolatePath, args...).CombinedOutput()
	compileOutput := readFileIfExists(paths.CompileOutputPath)
	if compileOutput != "" {
		job.Output.CompileOutput = compileOutput
	}

	if err != nil {
		if compileOutput == "" {
			job.Output.CompileOutput = strings.TrimSpace(string(output))
		}
		return core.JobStatus{Kind: core.StatusCompilationError}, nil
	}

	return core.JobStatus{Kind: core.StatusAccepted}, nil
}

func runJob(ctx context.Context, job *core.Job, boxID uint64, paths jobPaths) error {
	parts := strings.Fields(job.Language.RunCmd)
	if len(parts) == 0 {
		return errors.New("run command is empty")
	}
	runExe := parts[0]
	runArgs := strings.Join(parts[1:], " ")
	cmdStr := fmt.Sprintf("%s %s > /box/stdout 2> /box/stderr", runExe, runArgs)

	args := []string{
		"--cg",
		"-b", strconv.FormatUint(boxID, 10),
		"-M", paths.MetadataPath,
		fmt.Sprintf("--process=%d", job.Settings.MaxProcesses),
		"-t", fmt.Sprintf("%g", job.Settings.CPUTimeLimit),
		"-x", "0",
		"-w", fmt.Sprintf("%g", job.Settings.WallTimeLimit),
		"-k", fmt.Sprintf("%d", job.Settings.StackLimit),
		"-f", fmt.Sprintf("%d", job.Settings.MaxFileSize),
		fmt.Sprintf("--cg-mem=%d", job.Settings.MemoryLimit),
		"-E", "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"-E", "HOME=/tmp",
		"-d", "/etc:noexec",
		"--run",
		"--",
		"/usr/bin/sh",
		"-c",
		cmdStr,
	}

	cmd := exec.CommandContext(ctx, isolatePath, args...)
	stdinFile, err := os.Open(paths.StdinPath)
	if err != nil {
		return fmt.Errorf("open stdin: %w", err)
	}
	defer stdinFile.Close()
	cmd.Stdin = stdinFile

	output, err := cmd.CombinedOutput()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return nil
		}
		return fmt.Errorf("isolate run failed: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func readOutputs(job *core.Job, paths jobPaths) error {
	job.Output.Stdout = readFileIfExists(paths.StdoutPath)
	job.Output.Stderr = readFileIfExists(paths.StderrPath)
	if job.Output.CompileOutput == "" {
		job.Output.CompileOutput = readFileIfExists(paths.CompileOutputPath)
	}
	return nil
}

type metadata struct {
	Time     float64
	Memory   uint64
	ExitCode int
	Message  string
	Status   string
}

func readMetadata(path string) (metadata, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return metadata{}, fmt.Errorf("read metadata: %w", err)
	}
	lines := strings.Split(string(content), "\n")
	meta := metadata{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		switch key {
		case "time":
			meta.Time, _ = strconv.ParseFloat(value, 64)
		case "max-rss", "cg-mem":
			mem, _ := strconv.ParseUint(value, 10, 64)
			if mem > meta.Memory {
				meta.Memory = mem
			}
		case "exitcode":
			exit, _ := strconv.Atoi(value)
			meta.ExitCode = exit
		case "message":
			meta.Message = value
		case "status":
			meta.Status = value
		}
	}
	return meta, nil
}

func determineStatus(status string, exitCode int, stdout, expected string) core.JobStatus {
	switch status {
	case "TO":
		return core.JobStatus{Kind: core.StatusTimeLimitExceeded}
	case "SG":
		return core.RuntimeErrorStatus(runtimeSignal(exitCode))
	case "RE":
		return core.RuntimeErrorStatus("NZEC")
	case "XX":
		return core.JobStatus{Kind: core.StatusInternalError}
	default:
		if expected == "" || strings.TrimSpace(stdout) == strings.TrimSpace(expected) {
			return core.JobStatus{Kind: core.StatusAccepted}
		}
		return core.JobStatus{Kind: core.StatusWrongAnswer}
	}
}

func runtimeSignal(exitCode int) string {
	switch exitCode {
	case 11:
		return "SIGSEGV"
	case 25:
		return "SIGXFSZ"
	case 8:
		return "SIGFPE"
	case 6:
		return "SIGABRT"
	default:
		return "Other"
	}
}

func readFileIfExists(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(content)
}

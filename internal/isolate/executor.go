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
	"sync"
	"time"

	"flash-go/internal/models"
	"flash-go/internal/utils"
)

const (
	isolatePath = "isolate"
	boxModulo   = 2147483647
)
var useCgroup = utils.DetectCgroupSupport()

type boxHandle struct {
	id   uint64
	path string
	mu   sync.Mutex
}

func (b *boxHandle) initIfNeeded(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.path != "" {
		return nil
	}
	boxPath, err := initBox(ctx, b.id)
	if err != nil {
		return err
	}
	b.path = boxPath
	return nil
}

type Executor struct {
	pool    chan *boxHandle
	usePool bool
}

// NewExecutor creates an isolate executor with a reusable box pool.
func NewExecutor(poolSize int, usePool bool) *Executor {
	executor := &Executor{usePool: usePool}
	if !usePool {
		return executor
	}
	if poolSize < 1 {
		poolSize = 1
	}
	pool := make(chan *boxHandle, poolSize)
	for i := 0; i < poolSize; i++ {
		pool <- &boxHandle{id: uint64(i + 1)}
	}
	executor.pool = pool
	return executor
}

func (e *Executor) acquireBox(ctx context.Context) (*boxHandle, error) {
	if !e.usePool || e.pool == nil {
		return nil, errors.New("executor pool is not enabled")
	}
	select {
	case box := <-e.pool:
		if err := box.initIfNeeded(ctx); err != nil {
			e.pool <- box
			return nil, err
		}
		return box, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (e *Executor) releaseBox(box *boxHandle) {
	if box == nil || e.pool == nil {
		return
	}
	e.pool <- box
}

func (e *Executor) Execute(ctx context.Context, job *models.Job) (models.JobStatus, error) {

	var (
		boxID   uint64
		boxPath string
		box     *boxHandle
		err     error
	)
	if e.usePool {
		box, err = e.acquireBox(ctx)
		if err != nil {
			job.Status = models.JobStatus{Kind: models.StatusInternalError}
			job.Output.Message = err.Error()
			job.FinishedAt = time.Now().UnixNano()
			return job.Status, err
		}
		defer e.releaseBox(box)

		if err := cleanBoxContents(box.path); err != nil {
			job.Status = models.JobStatus{Kind: models.StatusInternalError}
			job.Output.Message = fmt.Sprintf("failed to clean box: %v", err)
			job.FinishedAt = time.Now().UnixNano()
			return job.Status, err
		}

		boxID = box.id
		boxPath = box.path
	} else {
		boxID = job.ID % boxModulo
		boxPath, err = initBox(ctx, boxID)
		if err != nil {
			job.Status = models.JobStatus{Kind: models.StatusInternalError}
			job.Output.Message = err.Error()
			job.FinishedAt = time.Now().UnixNano()
			return job.Status, err
		}
	}

	paths, err := setupFiles(job, boxPath)
	if err != nil {
		job.Status = models.JobStatus{Kind: models.StatusInternalError}
		job.Output.Message = err.Error()
		job.FinishedAt = time.Now().UnixNano()
		return job.Status, err
	}

	if job.Language.CompileCmd != "" {
		compileStatus, compileErr := compileJob(ctx, job, boxID, paths)
		if compileErr != nil {
			job.Status = models.JobStatus{Kind: models.StatusInternalError}
			job.Output.Message = compileErr.Error()
			job.FinishedAt = time.Now().UnixNano()
			return job.Status, compileErr
		}
		if compileStatus.Kind == models.StatusCompilationError {
			job.Status = compileStatus
			job.FinishedAt = time.Now().UnixNano()
			return job.Status, nil
		}
	}

	runErr := runJob(ctx, job, boxID, paths)
	if runErr != nil && !errors.Is(runErr, context.DeadlineExceeded) {
		job.Status = models.JobStatus{Kind: models.StatusInternalError}
		job.Output.Message = runErr.Error()
		job.FinishedAt = time.Now().UnixNano()
		return job.Status, runErr
	}

	if err := readOutputs(job, paths); err != nil {
		job.Status = models.JobStatus{Kind: models.StatusInternalError}
		job.Output.Message = err.Error()
		job.FinishedAt = time.Now().UnixNano()
		return job.Status, err
	}

	meta, err := utils.ReadMetadata(paths.MetadataPath)
	if err != nil {
		job.Status = models.JobStatus{Kind: models.StatusInternalError}
		job.Output.Message = err.Error()
		job.FinishedAt = time.Now().UnixNano()
		return job.Status, err
	}

	job.Output.Time = meta.Time
	job.Output.Memory = meta.Memory
	job.Output.ExitCode = meta.ExitCode
	job.Output.Message = meta.Message

	job.Status = utils.DetermineStatus(meta.Status, meta.ExitCode, job.Output.Stdout, job.ExpectedOutput)
	job.FinishedAt = time.Now().UnixNano()

	return job.Status, nil
}

func (e *Executor) Cleanup(jobID uint64) {
	if e.usePool {
		return
	}
	boxID := jobID % boxModulo
	boxIDStr := strconv.FormatUint(boxID, 10)
	
	args := []string{"-b", boxIDStr}
	if useCgroup {
		args = append([]string{"--cg"}, args...)
	}
	args = append(args, "--cleanup")
	
	cmd := exec.Command(isolatePath, args...)
	_ = cmd.Start()
	go func() {
		_ = cmd.Wait()
	}()
}

func (e *Executor) CleanupSync(jobID uint64) {
	if e.usePool {
		return
	}
	boxID := jobID % boxModulo
	
	args := []string{"-b", strconv.FormatUint(boxID, 10)}
	if useCgroup {
		args = append([]string{"--cg"}, args...)
	}
	args = append(args, "--cleanup")
	
	_ = exec.Command(isolatePath, args...).Run()
}

func initBox(ctx context.Context, boxID uint64) (string, error) {
	args := []string{"-b", strconv.FormatUint(boxID, 10), "--init"}
	if useCgroup {
		args = append([]string{"--cg"}, args...)
	}
	
	cmd := exec.CommandContext(ctx, isolatePath, args...)
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

// cleanBoxContents removes all files and directories inside the box, but keeps the box structure intact.
func cleanBoxContents(boxPath string) error {
	boxDir := filepath.Join(boxPath, "box")

	if _, err := os.Stat(boxDir); os.IsNotExist(err) {
		return nil
	}

	entries, err := os.ReadDir(boxDir)
	if err != nil {
		return fmt.Errorf("read box directory: %w", err)
	}

	for _, entry := range entries {
		entryPath := filepath.Join(boxDir, entry.Name())
		if err := os.RemoveAll(entryPath); err != nil {
			return fmt.Errorf("remove %s: %w", entryPath, err)
		}
	}

	return nil
}

func setupFiles(job *models.Job, boxPath string) (models.JobPaths, error) {
	boxDir := filepath.Join(boxPath, "box")
	sourcePath := filepath.Join(boxDir, job.Language.SourceFile)
	stdinPath := filepath.Join(boxDir, "stdin")
	stdoutPath := filepath.Join(boxDir, "stdout")
	stderrPath := filepath.Join(boxDir, "stderr")
	metadataPath := filepath.Join(boxDir, "metadata")
	compileOutputPath := filepath.Join(boxDir, "compile_output")

	if err := os.WriteFile(sourcePath, []byte(job.SourceCode), 0o644); err != nil {
		return models.JobPaths{}, fmt.Errorf("write source: %w", err)
	}
	if err := os.WriteFile(stdinPath, []byte(job.Stdin), 0o644); err != nil {
		return models.JobPaths{}, fmt.Errorf("write stdin: %w", err)
	}

	return models.JobPaths{
		BoxPath:           boxPath,
		MetadataPath:      metadataPath,
		StdoutPath:        stdoutPath,
		StderrPath:        stderrPath,
		StdinPath:         stdinPath,
		CompileOutputPath: compileOutputPath,
	}, nil
}

// getCgroupFlags returns cgroup-related flags based on job settings
func getCgroupFlags(job *models.Job) []string {
	flags := []string{}


	if !useCgroup {
		flags = append(flags, "-m", strconv.FormatUint(job.Settings.MemoryLimit, 10))
		return flags
	}

	if job.Settings.EnablePerProcessAndThreadMemoryLimit {
		flags = append(flags, "-m", strconv.FormatUint(job.Settings.MemoryLimit, 10))
	} else {
		flags = append(flags, "--cg-mem="+strconv.FormatUint(job.Settings.MemoryLimit, 10))
	}
	
	return flags
}

func compileJob(ctx context.Context, job *models.Job, boxID uint64, paths models.JobPaths) (models.JobStatus, error) {
	parts := strings.Fields(job.Language.CompileCmd)
	if len(parts) == 0 {
		return models.JobStatus{Kind: models.StatusInternalError}, errors.New("compile command is empty")
	}

	sb := utils.GetStringBuilder()
	sb.WriteString(parts[0])
	for i := 1; i < len(parts); i++ {
		sb.WriteByte(' ')
		sb.WriteString(parts[i])
	}
	sb.WriteString(" 2> /box/compile_output")
	cmdStr := sb.String()
	utils.PutStringBuilder(sb)

	boxIDStr := strconv.FormatUint(boxID, 10)
	processStr := strconv.FormatUint(uint64(job.Settings.MaxProcesses), 10)
	cpuTimeStr := strconv.FormatFloat(job.Settings.MaxCPUTimeLimit, 'g', -1, 64)
	wallTimeStr := strconv.FormatFloat(job.Settings.MaxWallTimeLimit, 'g', -1, 64)
	stackStr := strconv.FormatUint(job.Settings.MaxStackLimit, 10)
	fileSizeStr := strconv.FormatUint(job.Settings.MaxFileSize, 10)

	args := make([]string, 0, 30)
	
	if useCgroup {
		args = append(args, "--cg")
	}
	
	args = append(args,
		"-s",
		"-b", boxIDStr,
		"-M", paths.MetadataPath,
		"--stderr-to-stdout",
		"-i", "/dev/null",
		"--process="+processStr,
		"-t", cpuTimeStr,
		"-x", "0",
		"-w", wallTimeStr,
		"-k", stackStr,
		"-f", fileSizeStr,
		"-E", "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"-E", "HOME=/tmp",
		"-d", "/etc:noexec",
	)
	
	// Add cgroup-specific flags
	cgFlags := getCgroupFlags(job)
	args = append(args, cgFlags...)
	
	args = append(args,
		"--run",
		"--",
		"/usr/bin/sh",
		"-c",
		cmdStr,
	)

	output, err := exec.CommandContext(ctx, isolatePath, args...).CombinedOutput()
	compileOutput := utils.ReadFileIfExists(paths.CompileOutputPath)
	if compileOutput != "" {
		job.Output.CompileOutput = compileOutput
	}

	if err != nil {
		if compileOutput == "" {
			job.Output.CompileOutput = strings.TrimSpace(string(output))
		}
		if job.Output.CompileOutput != "" {
			job.Output.Message = job.Output.CompileOutput
		} else {
			job.Output.Message = "Compilation failed"
		}
		return models.JobStatus{Kind: models.StatusCompilationError}, nil
	}

	return models.JobStatus{Kind: models.StatusAccepted}, nil
}

func runJob(ctx context.Context, job *models.Job, boxID uint64, paths models.JobPaths) error {
	parts := strings.Fields(job.Language.RunCmd)
	if len(parts) == 0 {
		return errors.New("run command is empty")
	}

	sb := utils.GetStringBuilder()
	sb.WriteString(parts[0])
	for i := 1; i < len(parts); i++ {
		sb.WriteByte(' ')
		sb.WriteString(parts[i])
	}
	sb.WriteString(" > /box/stdout 2> /box/stderr")
	cmdStr := sb.String()
	utils.PutStringBuilder(sb)

	boxIDStr := strconv.FormatUint(boxID, 10)
	processStr := strconv.FormatUint(uint64(job.Settings.MaxProcesses), 10)
	cpuTimeStr := strconv.FormatFloat(job.Settings.CPUTimeLimit, 'g', -1, 64)
	wallTimeStr := strconv.FormatFloat(job.Settings.WallTimeLimit, 'g', -1, 64)
	stackStr := strconv.FormatUint(job.Settings.StackLimit, 10)
	fileSizeStr := strconv.FormatUint(job.Settings.MaxFileSize, 10)

	args := make([]string, 0, 30)
	
	if useCgroup {
		args = append(args, "--cg")
	}
	
	args = append(args,
		"-s",
		"-b", boxIDStr,
		"-M", paths.MetadataPath,
	)
	
	if job.Settings.RedirectStderrToStdout {
		args = append(args, "--stderr-to-stdout")
	}
	
	if job.Settings.EnableNetwork {
		args = append(args, "--share-net")
	}
	
	args = append(args,
		"--process="+processStr,
		"-t", cpuTimeStr,
		"-x", "0",
		"-w", wallTimeStr,
		"-k", stackStr,
		"-f", fileSizeStr,
		"-E", "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"-E", "HOME=/tmp",
		"-d", "/etc:noexec",
	)
	
	cgFlags := getCgroupFlags(job)
	args = append(args, cgFlags...)
	
	args = append(args,
		"--run",
		"--",
		"/usr/bin/sh",
		"-c",
		cmdStr,
	)

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

func readOutputs(job *models.Job, paths models.JobPaths) error {
	job.Output.Stdout = utils.ReadFileIfExists(paths.StdoutPath)
	job.Output.Stderr = utils.ReadFileIfExists(paths.StderrPath)
	if job.Output.CompileOutput == "" && job.Language.CompileCmd != "" {
		job.Output.CompileOutput = utils.ReadFileIfExists(paths.CompileOutputPath)
	} else if job.Language.CompileCmd == "" {
		job.Output.CompileOutput = ""
	}
	return nil
}
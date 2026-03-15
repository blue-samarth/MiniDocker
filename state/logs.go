package state

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

const (
	maxLogSize   = 10 * 1024 * 1024 // 10 MB per log file
	maxRotations = 3                // keep stdout, stdout.1, stdout.2, stdout.3
)

// LogManager handles stdout/stderr capture and log rotation for a container.
type LogManager struct {
	containerID string
	logDir      string
	stdoutFile  *os.File
	stderrFile  *os.File
}

// NewLogManager creates a LogManager for the given container,
// using the log directory from its state.
func NewLogManager(containerID, logDir string) (*LogManager, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log dir: %w", err)
	}

	stdoutPath := filepath.Join(logDir, "stdout")
	stderrPath := filepath.Join(logDir, "stderr")

	stdoutFile, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open stdout log: %w", err)
	}

	stderrFile, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		stdoutFile.Close()
		return nil, fmt.Errorf("failed to open stderr log: %w", err)
	}

	return &LogManager{
		containerID: containerID,
		logDir:      logDir,
		stdoutFile:  stdoutFile,
		stderrFile:  stderrFile,
	}, nil
}

// StdoutWriter returns an io.Writer that writes to the stdout log file.
func (lm *LogManager) StdoutWriter() io.Writer {
	return lm.stdoutFile
}

// StderrWriter returns an io.Writer that writes to the stderr log file.
func (lm *LogManager) StderrWriter() io.Writer {
	return lm.stderrFile
}

// CaptureOutput starts goroutines to copy from the provided readers into log files.
// Returns immediately; copying runs in the background.
func (lm *LogManager) CaptureOutput(stdout, stderr io.Reader) {
	if stdout != nil {
		go func() {
			if _, err := io.Copy(lm.stdoutFile, stdout); err != nil {
				log.Printf("[logs] stdout copy error for %s: %v", lm.containerID, err)
			}
		}()
	}
	if stderr != nil {
		go func() {
			if _, err := io.Copy(lm.stderrFile, stderr); err != nil {
				log.Printf("[logs] stderr copy error for %s: %v", lm.containerID, err)
			}
		}()
	}
}

// GetLogs returns the last n lines from the combined stdout log.
// If n <= 0, all lines are returned.
func (lm *LogManager) GetLogs(lines int) ([]byte, error) {
	stdoutPath := filepath.Join(lm.logDir, "stdout")
	data, err := os.ReadFile(stdoutPath)
	if os.IsNotExist(err) {
		return []byte{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read stdout log: %w", err)
	}

	if lines <= 0 {
		return data, nil
	}

	return tailLines(data, lines), nil
}

// GetStderrLogs returns the last n lines from the stderr log.
func (lm *LogManager) GetStderrLogs(lines int) ([]byte, error) {
	stderrPath := filepath.Join(lm.logDir, "stderr")
	data, err := os.ReadFile(stderrPath)
	if os.IsNotExist(err) {
		return []byte{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read stderr log: %w", err)
	}

	if lines <= 0 {
		return data, nil
	}

	return tailLines(data, lines), nil
}

// RotateLogs rotates the log files if they exceed maxLogSize.
func (lm *LogManager) RotateLogs() error {
	for _, name := range []string{"stdout", "stderr"} {
		path := filepath.Join(lm.logDir, name)
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("failed to stat %s log: %w", name, err)
		}

		if info.Size() < maxLogSize {
			continue
		}

		if err := rotate(path); err != nil {
			log.Printf("[logs] warning: failed to rotate %s for %s: %v", name, lm.containerID, err)
		}
	}
	return nil
}

// Close closes the underlying log files.
func (lm *LogManager) Close() error {
	var errs []error
	if lm.stdoutFile != nil {
		if err := lm.stdoutFile.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if lm.stderrFile != nil {
		if err := lm.stderrFile.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors closing log files: %v", errs)
	}
	return nil
}

// rotate renames path → path.1, path.1 → path.2, etc., up to maxRotations.
func rotate(path string) error {
	// Remove oldest rotation
	oldest := fmt.Sprintf("%s.%d", path, maxRotations)
	os.Remove(oldest)

	// Shift existing rotations
	for i := maxRotations - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", path, i)
		dst := fmt.Sprintf("%s.%d", path, i+1)
		if _, err := os.Stat(src); err == nil {
			os.Rename(src, dst)
		}
	}

	// Rename current log to .1
	return os.Rename(path, path+".1")
}

// tailLines returns the last n newline-separated lines from data.
func tailLines(data []byte, n int) []byte {
	if len(data) == 0 {
		return data
	}

	// Find newline positions from the end
	end := len(data)
	// Strip trailing newline for counting
	if data[end-1] == '\n' {
		end--
	}

	count := 0
	pos := end
	for pos > 0 {
		pos--
		if data[pos] == '\n' {
			count++
			if count == n {
				return data[pos+1:]
			}
		}
	}
	return data[:end+1]
}

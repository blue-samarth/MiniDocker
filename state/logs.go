package state

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
)

const (
	maxLogSize   = 10 * 1024 * 1024 // 10 MB per log file
	maxRotations = 3
)

// LogManager handles stdout/stderr capture and log rotation for a container.
type LogManager struct {
	containerID string
	logDir      string
	stdoutFile  *os.File
	stderrFile  *os.File
}

func NewLogManager(containerID, logDir string) (*LogManager, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}

	stdoutPath := filepath.Join(logDir, "stdout")
	stderrPath := filepath.Join(logDir, "stderr")

	stdoutFile, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	stderrFile, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		stdoutFile.Close()
		return nil, err
	}

	return &LogManager{
		containerID: containerID,
		logDir:      logDir,
		stdoutFile:  stdoutFile,
		stderrFile:  stderrFile,
	}, nil
}

func (lm *LogManager) StdoutWriter() io.Writer {
	return lm.stdoutFile
}

func (lm *LogManager) StderrWriter() io.Writer {
	return lm.stderrFile
}

func (lm *LogManager) CaptureOutput(stdout, stderr io.Reader) {
	if stdout != nil {
		go func() {
			_, err := io.Copy(lm.stdoutFile, stdout)
			if err != nil && err != io.EOF {
				log.Printf("stdout copy failed for container %s: %v", lm.containerID, err)
			}
		}()
	}
	if stderr != nil {
		go func() {
			_, err := io.Copy(lm.stderrFile, stderr)
			if err != nil && err != io.EOF {
				log.Printf("stderr copy failed for container %s: %v", lm.containerID, err)
			}
		}()
	}
}

func (lm *LogManager) GetLogs(lines int) ([]byte, error) {
	path := filepath.Join(lm.logDir, "stdout")
	return readTail(path, lines)
}

func (lm *LogManager) GetStderrLogs(lines int) ([]byte, error) {
	path := filepath.Join(lm.logDir, "stderr")
	return readTail(path, lines)
}

func (lm *LogManager) RotateLogs() error {
	for _, name := range []string{"stdout", "stderr"} {
		path := filepath.Join(lm.logDir, name)
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}

		if info.Size() < maxLogSize {
			continue
		}

		if err := rotate(path); err != nil {
			log.Printf("failed to rotate %s log for container %s: %v", name, lm.containerID, err)
		}
	}
	return nil
}

func (lm *LogManager) Close() error {
	if lm.stdoutFile != nil {
		_ = lm.stdoutFile.Close()
	}
	if lm.stderrFile != nil {
		_ = lm.stderrFile.Close()
	}
	return nil
}

func readTail(path string, n int) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []byte{}, nil
		}
		return nil, err
	}
	if n <= 0 {
		return data, nil
	}
	return tailLines(data, n), nil
}

func rotate(path string) error {
	_ = os.Remove(path + "." + strconv.Itoa(maxRotations))

	for i := maxRotations - 1; i >= 1; i-- {
		src := path + "." + strconv.Itoa(i)
		dst := path + "." + strconv.Itoa(i+1)
		if _, err := os.Stat(src); err == nil {
			_ = os.Rename(src, dst)
		}
	}

	return os.Rename(path, path+".1")
}

func tailLines(data []byte, n int) []byte {
	if len(data) == 0 {
		return data
	}

	end := len(data)
	if end > 0 && data[end-1] == '\n' {
		end--
	}

	count := 0
	for i := end - 1; i >= 0; i-- {
		if data[i] == '\n' {
			count++
			if count == n {
				return data[i+1:]
			}
		}
	}
	return data[:end+1]
}

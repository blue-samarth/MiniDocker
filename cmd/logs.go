package cmd

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"golang.org/x/sys/unix"

	"miniDocker/state"
)

func Logs(args []string) error {
	fs := flag.NewFlagSet("logs", flag.ContinueOnError)
	follow := fs.Bool("f", false, "Follow log output (running containers only)")
	tail := fs.Int("tail", 0, "Show last N lines (0 = all)")
	timestamps := fs.Bool("timestamps", false, "Prefix each line with a timestamp")
	onlyStderr := fs.Bool("stderr", false, "Show only stderr")
	onlyStdout := fs.Bool("stdout", false, "Show only stdout")

	if err := fs.Parse(args); err != nil {
		return err
	}

	ids := fs.Args()
	if len(ids) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: miniDocker logs [options] <container-id>")
		return fmt.Errorf("container ID required")
	}

	lm, err := getLifecycleManager()
	if err != nil {
		return err
	}

	cs, err := loadContainer(lm, ids[0])
	if err != nil {
		return err
	}

	logDir, err := lm.GetLogDir(ids[0])
	if err != nil {
		return fmt.Errorf("failed to get log directory: %w", err)
	}

	// Default: show both streams unless one is explicitly selected.
	wantOut := !*onlyStderr || *onlyStdout
	wantErr := !*onlyStdout || *onlyStderr

	opts := logOpts{
		tail:       *tail,
		timestamps: *timestamps,
		wantStdout: wantOut,
		wantStderr: wantErr,
	}

	if *follow && cs.Status == state.StatusRunning {
		return followLogs(logDir, opts)
	}

	return printLogs(ids[0], logDir, opts)
}

type logOpts struct {
	tail       int
	timestamps bool
	wantStdout bool
	wantStderr bool
}

func printLogs(containerID, logDir string, opts logOpts) error {
	lm, err := state.NewLogManager(containerID, logDir)
	if err != nil {
		return fmt.Errorf("failed to open logs: %w", err)
	}
	defer lm.Close()

	if opts.wantStdout {
		data, err := lm.GetLogs(opts.tail)
		if err != nil {
			return fmt.Errorf("failed to read stdout logs: %w", err)
		}
		writeLogData(data, opts.timestamps)
	}

	if opts.wantStderr {
		data, err := lm.GetStderrLogs(opts.tail)
		if err != nil {
			return fmt.Errorf("failed to read stderr logs: %w", err)
		}
		writeLogData(data, opts.timestamps)
	}

	return nil
}

func writeLogData(data []byte, timestamps bool) {
	if len(data) == 0 {
		return
	}
	if !timestamps {
		os.Stdout.Write(data)
		return
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		fmt.Printf("%s %s\n", time.Now().UTC().Format(time.RFC3339), scanner.Text())
	}
}

func followLogs(logDir string, opts logOpts) error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, unix.SIGINT, unix.SIGTERM)
	defer signal.Stop(sigCh)

	var stdoutFile, stderrFile *os.File

	if opts.wantStdout {
		f, err := os.Open(logDir + "/stdout")
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to open stdout log: %w", err)
		}
		if f != nil {
			defer f.Close()
			f.Seek(0, io.SeekEnd)
			stdoutFile = f
		}
	}

	if opts.wantStderr {
		f, err := os.Open(logDir + "/stderr")
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to open stderr log: %w", err)
		}
		if f != nil {
			defer f.Close()
			f.Seek(0, io.SeekEnd)
			stderrFile = f
		}
	}

	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	buf := make([]byte, 32*1024)

	for {
		select {
		case <-sigCh:
			return nil
		case <-tick.C:
			if stdoutFile != nil {
				drainFile(stdoutFile, buf, os.Stdout)
			}
			if stderrFile != nil {
				drainFile(stderrFile, buf, os.Stderr)
			}
		}
	}
}

func drainFile(f *os.File, buf []byte, dst *os.File) {
	for {
		n, err := f.Read(buf)
		if n > 0 {
			dst.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
}

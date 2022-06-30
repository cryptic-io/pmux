package pmuxlib

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ProcessConfig is used to configure a process via RunProcess.
type ProcessConfig struct {

	// Name of the process to be run. This only gets used by RunPmux.
	Name string

	// Cmd and Args describe the actual process to run.
	Cmd  string   `yaml:"cmd"`
	Args []string `yaml:"args"`

	// Env describes the environment variables to set on the process.
	Env map[string]string `yaml:"env"`

	// Dir is the directory the process will be run in. If not set then the
	// process is run in the same directory as this parent process.
	Dir string `yaml:"dir"`

	// MinWait and MaxWait are the minimum and maximum amount of time between
	// restarts that RunProcess will wait.
	//
	// MinWait defaults to 1 second.
	// MaxWait defaults to 64 seconds.
	MinWait time.Duration `yaml:"minWait"`
	MaxWait time.Duration `yaml:"maxWait"`

	// SigKillWait is the amount of time after the process is sent a SIGINT
	// before RunProcess sends it a SIGKILL.
	//
	// Defalts to 10 seconds.
	SigKillWait time.Duration `yaml:"sigKillWait"`

	// NoRestartOn indicates which exit codes should result in the process not
	// being restarted any further.
	NoRestartOn []int `yaml:"noRestartOn"`
}

func (cfg ProcessConfig) withDefaults() ProcessConfig {

	if cfg.MinWait == 0 {
		cfg.MinWait = 1 * time.Second
	}

	if cfg.MaxWait == 0 {
		cfg.MaxWait = 64 * time.Second
	}

	if cfg.SigKillWait == 0 {
		cfg.SigKillWait = 10 * time.Second
	}

	return cfg
}

func sigProcessGroup(sysLogger Logger, proc *os.Process, sig syscall.Signal) {
	sysLogger.Printf("sending %v signal", sig)

	// Because we use Setpgid when starting child processes, child processes
	// will have the same PGID as their PID. To send a signal to all processes
	// in a group, you send the signal to the negation of the PGID, which in
	// this case is equivalent to -PID.
	//
	// POSIX is a fucking joke.
	if err := syscall.Kill(-proc.Pid, sig); err != nil {

		panic(fmt.Errorf(
			"failed to send %v signal to %d: %w",
			sig, -proc.Pid, err,
		))
	}
}

// RunProcessOnce runs the process described by the ProcessConfig (though it
// doesn't use all fields from the ProcessConfig).
//
// The process is killed if-and-only-if the context is canceled, returning -1
// and context.Canceled. Otherwise the exit status of the process is returned,
// or -1 and an error.
//
// The stdout and stderr of the process will be written to the corresponding
// Loggers. Various runtime events will be written to the sysLogger.
func RunProcessOnce(
	ctx context.Context,
	stdoutLogger, stderrLogger, sysLogger Logger,
	cfg ProcessConfig,
) (
	int, error,
) {

	cfg = cfg.withDefaults()

	var wg sync.WaitGroup

	fwdOutPipe := func(logger Logger, r io.Reader) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bufR := bufio.NewReader(r)
			for {
				line, err := bufR.ReadString('\n')
				if errors.Is(err, io.EOF) {
					return
				} else if err != nil {
					logger.Printf("reading output: %v", err)
					return
				}

				logger.Println(strings.TrimSuffix(line, "\n"))
			}
		}()
	}

	cmd := exec.Command(cfg.Cmd, cfg.Args...)

	cmd.Dir = cfg.Dir

	cmd.SysProcAttr = &syscall.SysProcAttr{
		// Indicates that the child process should be a part of a separate
		// process group than the parent, so that it does not receive signals
		// that the parent receives. This is what ensures that context
		// cancellation is the only way to interrupt the child processes.
		Setpgid: true,
	}

	cmd.Env = os.Environ()
	for k, v := range cfg.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return -1, fmt.Errorf("getting stdout pipe: %w", err)
	}
	defer stdout.Close()

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return -1, fmt.Errorf("getting stderr pipe: %w", err)
	}
	defer stderr.Close()

	fwdOutPipe(stdoutLogger, stdout)
	fwdOutPipe(stderrLogger, stderr)

	if err := cmd.Start(); err != nil {
		return -1, fmt.Errorf("starting process: %w", err)
	}

	stopCh := make(chan struct{})

	go func(proc *os.Process) {

		select {
		case <-ctx.Done():
			sigProcessGroup(sysLogger, proc, syscall.SIGINT)
		case <-stopCh:
			return
		}

		select {
		case <-time.After(cfg.SigKillWait):
			sigProcessGroup(sysLogger, proc, syscall.SIGKILL)
		case <-stopCh:
		}

	}(cmd.Process)

	wg.Wait()

	err = cmd.Wait()
	close(stopCh)

	if err := ctx.Err(); err != nil {
		return -1, err
	}

	if err != nil {
		return -1, fmt.Errorf("process exited: %w", err)
	}

	return cmd.ProcessState.ExitCode(), nil
}

// RunProcess runs a process (configured by ProcessConfig) until the context is
// canceled, at which point the process is killed and RunProcess returns.
//
// The process will be restarted if it exits of its own accord. There will be a
// brief wait time between each restart, with an exponential backoff mechanism
// so that the wait time increases upon repeated restarts.
//
// The stdout and stderr of the process will be written to the corresponding
// Loggers. Various runtime events will be written to the sysLogger.
func RunProcess(
	ctx context.Context,
	stdoutLogger, stderrLogger, sysLogger Logger,
	cfg ProcessConfig,
) {

	cfg = cfg.withDefaults()

	var wait time.Duration

	for {
		start := time.Now()
		exitCode, err := RunProcessOnce(
			ctx,
			stdoutLogger, stderrLogger, sysLogger,
			cfg,
		)
		took := time.Since(start)

		if err != nil {
			sysLogger.Printf("exited: %v", err)
		} else {
			sysLogger.Printf("exit code: %d", exitCode)
		}

		if err := ctx.Err(); err != nil {
			return
		}

		for i := range cfg.NoRestartOn {
			if cfg.NoRestartOn[i] == exitCode {
				return
			}
		}

		wait = ((wait * 2) - took).Truncate(time.Millisecond)

		if wait < cfg.MinWait {
			wait = cfg.MinWait
		} else if wait > cfg.MaxWait {
			wait = cfg.MaxWait
		}

		sysLogger.Printf("will restart process in %v", wait)

		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return
		}
	}
}

// Package pmuxproc implements the process management aspects of the pmux
// process.
package pmuxproc

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
	"time"
)

// Config is used to configure a process via RunProcess.
type Config struct {

	// Cmd and Args describe the actual process to run.
	Cmd  string   `yaml:"cmd"`
	Args []string `yaml:"args"`

	// Env describes the environment variables to set on the process.
	Env map[string]string `yaml:"env"`

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
}

func (cfg Config) withDefaults() Config {

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

func runProcessOnce(ctx context.Context, logger Logger, cfg Config) error {

	var wg sync.WaitGroup

	fwdOutPipe := func(r io.Reader) {
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

	cmd.Env = make([]string, 0, len(cfg.Env))
	for k, v := range cfg.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("getting stdout pipe: %w", err)
	}
	defer stdout.Close()

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("getting stderr pipe: %w", err)
	}
	defer stderr.Close()

	fwdOutPipe(stdout)
	fwdOutPipe(stderr)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting process: %w", err)
	}

	// go-routine which will sent interrupt if the context is cancelled. Also
	// waits on a secondary channel, which is closed when this function returns,
	// in order to ensure this go-routine always gets cleaned up.
	stopCh := make(chan struct{})
	defer close(stopCh)
	go func(proc *os.Process) {
		select {
		case <-ctx.Done():
			_ = proc.Signal(os.Interrupt)
		case <-stopCh:
			return
		}

		select {
		case <-time.After(cfg.SigKillWait):
			logger.Println("forcefully killing process")
			_ = proc.Signal(os.Kill)
		case <-stopCh:
			return
		}
	}(cmd.Process)

	wg.Wait()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("process exited: %w", err)
	}

	return nil
}

// RunProcess is a process (configured by Config) until the context is canceled,
// at which point the process is killed and RunProcess returns.
//
// The process will be restarted if it exits of its own accord. There will be a
// brief wait time between each restart, with an exponential backup mechanism so
// that the wait time increases upon repeated restarts.
//
// The stdout and stderr of the process will be written to the given Logger, as
// well as various runtime events.
func RunProcess(ctx context.Context, logger Logger, cfg Config) {

	cfg = cfg.withDefaults()

	var wait time.Duration

	for {
		start := time.Now()
		err := runProcessOnce(ctx, logger, cfg)
		took := time.Since(start)

		if err != nil {
			logger.Printf("%v", err)
		} else {
			logger.Println("exit status 0")
		}

		if err := ctx.Err(); err != nil {
			return
		}

		wait = ((wait * 2) - took).Truncate(time.Millisecond)

		if wait < cfg.MinWait {
			wait = cfg.MinWait
		} else if wait > cfg.MaxWait {
			wait = cfg.MaxWait
		}

		logger.Printf("will restart process in %v", wait)

		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return
		}
	}
}
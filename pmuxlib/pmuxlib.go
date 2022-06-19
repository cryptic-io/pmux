// Package pmuxlib implements the process management aspects of the pmux
// process.
package pmuxlib

import (
	"context"
	"os"
	"sync"
)

type Config struct {
	TimeFormat string          `yaml:"timeFormat"`
	Processes  []ProcessConfig `yaml:"processes"`
}

// Run runs the given configuration as if this was a real pmux process.
func Run(ctx context.Context, cfg Config) {

	stdoutLogger := newLogger(os.Stdout, logSepStdout, cfg.TimeFormat)
	defer stdoutLogger.Close()

	stderrLogger := newLogger(os.Stderr, logSepStderr, cfg.TimeFormat)
	defer stderrLogger.Close()

	sysLogger := stderrLogger.withSep(logSepSys)
	defer sysLogger.Println("exited gracefully, ciao!")

	var wg sync.WaitGroup
	defer wg.Wait()

	for _, cfgProc := range cfg.Processes {
		wg.Add(1)
		go func(procCfg ProcessConfig) {
			defer wg.Done()

			stdoutLogger := stdoutLogger.withPName(procCfg.Name)
			stderrLogger := stderrLogger.withPName(procCfg.Name)
			sysLogger := sysLogger.withPName(procCfg.Name)

			sysLogger.Println("starting process")
			defer sysLogger.Println("stopped process handler")

			RunProcess(
				ctx, stdoutLogger, stderrLogger, sysLogger, procCfg,
			)

		}(cfgProc)
	}
}

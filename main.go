package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"gopkg.in/yaml.v2"
)

// pname used by pmux itself for logging.
const pmuxPName = "pmux"

// characters used to denote different kinds of logs
const (
	logSepProcOut = '>'
	logSepSys     = '|'
)

type logger struct {
	timeFmt     string
	maxPNameLen int

	l      sync.Mutex
	out    io.Writer
	outBuf *bufio.Writer

	wg      sync.WaitGroup
	closeCh chan struct{}
}

func newLogger(
	out io.Writer,
	timeFmt string,
	possiblePNames []string,
) *logger {

	maxPNameLen := 0
	for _, pname := range possiblePNames {
		if l := len(pname); l > maxPNameLen {
			maxPNameLen = l
		}
	}

	maxPNameLen++

	l := &logger{
		timeFmt:     timeFmt,
		maxPNameLen: maxPNameLen,
		out:         out,
		outBuf:      bufio.NewWriter(out),
		closeCh:     make(chan struct{}),
	}

	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		l.flusher()
	}()

	return l
}

func (l *logger) Close() {

	l.l.Lock()
	defer l.l.Unlock()

	close(l.closeCh)
	l.wg.Wait()

	l.outBuf.Flush()

	if syncer, ok := l.out.(interface{ Sync() error }); ok {
		syncer.Sync()
	} else if flusher, ok := l.out.(interface{ Flush() error }); ok {
		flusher.Flush()
	}

	// this generally shouldn't be necessary, but we could run into cases (e.g.
	// during a force-kill) where further Prints are called after a Close. These
	// should just do nothing.
	l.out = ioutil.Discard
	l.outBuf = bufio.NewWriter(l.out)
}

func (l *logger) flusher() {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.l.Lock()
			l.outBuf.Flush()
			l.l.Unlock()
		case <-l.closeCh:
			return
		}
	}
}

func (l *logger) Println(pname string, sep rune, line string) {

	l.l.Lock()
	defer l.l.Unlock()

	fmt.Fprintf(
		l.outBuf,
		"%s %c %s%s%c %s\n",
		time.Now().Format(l.timeFmt),
		sep,
		pname,
		strings.Repeat(" ", l.maxPNameLen-len(pname)),
		sep,
		line,
	)
}

func (l *logger) Printf(pname string, sep rune, msg string, args ...interface{}) {
	l.Println(pname, sep, fmt.Sprintf(msg, args...))
}

////////////////////////////////////////////////////////////////////////////////

type configProcess struct {
	Name        string            `yaml:"name"`
	Cmd         string            `yaml:"cmd"`
	Args        []string          `yaml:"args"`
	Env         map[string]string `yaml:"env"`
	MinWait     time.Duration     `yaml:"minWait"`
	MaxWait     time.Duration     `yaml:"maxWait"`
	SigKillWait time.Duration     `yaml:"sigKillWait"`
}

func runProcessOnce(
	ctx context.Context, logger *logger, cfgProc configProcess,
) error {

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
					logger.Printf(cfgProc.Name, logSepSys, "reading output: %v", err)
					return
				}

				logger.Println(cfgProc.Name, logSepProcOut, strings.TrimSuffix(line, "\n"))
			}
		}()
	}

	cmd := exec.Command(cfgProc.Cmd, cfgProc.Args...)

	cmd.Env = make([]string, 0, len(cfgProc.Env))
	for k, v := range cfgProc.Env {
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
			proc.Signal(os.Interrupt)
		case <-stopCh:
			return
		}

		select {
		case <-time.After(cfgProc.SigKillWait):
			logger.Println(cfgProc.Name, logSepSys, "forcefully killing process")
			proc.Signal(os.Kill)
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

func runProcess(
	ctx context.Context, logger *logger, cfgProc configProcess,
) {

	var wait time.Duration

	for {
		logger.Println(cfgProc.Name, logSepSys, "starting process")

		start := time.Now()
		err := runProcessOnce(ctx, logger, cfgProc)
		took := time.Since(start)

		if err != nil {
			logger.Printf(cfgProc.Name, logSepSys, "%v", err)
		} else {
			logger.Println(cfgProc.Name, logSepSys, "exit status 0")
		}

		if err := ctx.Err(); err != nil {
			return
		}

		wait = ((wait * 2) - took).Truncate(time.Millisecond)

		if wait < cfgProc.MinWait {
			wait = cfgProc.MinWait
		} else if wait > cfgProc.MaxWait {
			wait = cfgProc.MaxWait
		}

		logger.Printf(cfgProc.Name, logSepSys, "will restart process in %v", wait)
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return
		}
	}
}

////////////////////////////////////////////////////////////////////////////////

type config struct {
	TimeFormat string          `yaml:"timeFormat"`
	Processes  []configProcess `yaml:"processes"`
}

func (cfg config) init() (config, error) {
	if cfg.TimeFormat == "" {
		cfg.TimeFormat = "2006-01-02T15:04:05.000Z07:00"
	}

	if len(cfg.Processes) == 0 {
		return config{}, errors.New("no processes defined")
	}

	for i, cfgProc := range cfg.Processes {
		if cfgProc.MinWait == 0 {
			cfgProc.MinWait = 1 * time.Second
		}

		if cfgProc.MaxWait == 0 {
			cfgProc.MaxWait = 64 * time.Second
		}

		if cfgProc.SigKillWait == 0 {
			cfgProc.SigKillWait = 10 * time.Second
		}

		cfg.Processes[i] = cfgProc
	}

	return cfg, nil
}

func main() {

	cfgPath := flag.String("c", "./pmux.yml", "Path to config yaml file")
	flag.Parse()

	cfgB, err := ioutil.ReadFile(*cfgPath)
	if err != nil {
		panic(fmt.Sprintf("couldn't read cfg file at %q: %v", *cfgPath, err))
	}

	var cfg config
	if err := yaml.Unmarshal(cfgB, &cfg); err != nil {
		panic(fmt.Sprintf("couldn't parse cfg file: %v", err))

	} else if cfg, err = cfg.init(); err != nil {
		panic(fmt.Sprintf("initializing config: %v", err))
	}

	pnames := make([]string, len(cfg.Processes))
	for i, cfgProc := range cfg.Processes {
		pnames[i] = cfgProc.Name
	}

	logger := newLogger(os.Stdout, cfg.TimeFormat, pnames)
	defer logger.Close()
	defer logger.Println(pmuxPName, logSepSys, "exited gracefully, ciao!")

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		sigCh := make(chan os.Signal)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

		sig := <-sigCh
		logger.Printf(pmuxPName, logSepSys, "%v signal received, killing all sub-processes", sig)
		cancel()

		<-sigCh
		logger.Printf(pmuxPName, logSepSys, "forcefully exiting pmux process, there may be zombie child processes being left behind, good luck!")
		logger.Close()
		os.Exit(1)
	}()

	var wg sync.WaitGroup
	defer wg.Wait()

	for _, cfgProc := range cfg.Processes {
		wg.Add(1)
		go func(cfgProc configProcess) {
			defer wg.Done()
			runProcess(ctx, logger, cfgProc)
			logger.Printf(cfgProc.Name, logSepSys, "stopped process handler")
		}(cfgProc)
	}
}

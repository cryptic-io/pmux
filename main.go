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
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cryptic-io/pmux/pmuxproc"

	"gopkg.in/yaml.v2"
)

// pname used by pmux itself for logging.
const pmuxPName = "pmux"

// characters used to denote different kinds of logs
const (
	logSepStdout = '›'
	logSepStderr = '»'
	logSepSys    = '~'
)

type logger struct {
	timeFmt string

	l      *sync.Mutex
	out    io.Writer
	outBuf *bufio.Writer

	// maxPNameLen is a pointer because it changes when WithPrefix is called.
	maxPNameLen *uint64

	pname string
	sep   rune
}

func newLogger(
	out io.Writer,
	sep rune,
	timeFmt string,
) *logger {

	pname := pmuxPName
	maxPNameLen := uint64(len(pname))

	l := &logger{
		timeFmt:     timeFmt,
		maxPNameLen: &maxPNameLen,
		l:           new(sync.Mutex),
		out:         out,
		outBuf:      bufio.NewWriter(out),
		pname:       pname,
		sep:         sep,
	}

	return l
}

func (l *logger) withSep(sep rune) *logger {
	l2 := *l
	l2.sep = sep
	return &l2
}

func (l *logger) withPName(pname string) *logger {
	l2 := *l
	l2.pname = pname

	l2.l.Lock()
	defer l2.l.Unlock()

	if pnameLen := uint64(len(pname)); pnameLen > *l2.maxPNameLen {
		*l2.maxPNameLen = pnameLen
	}

	return &l2
}

func (l *logger) Close() {

	l.l.Lock()
	defer l.l.Unlock()

	l.outBuf.Flush()

	if syncer, ok := l.out.(interface{ Sync() error }); ok {
		_ = syncer.Sync()
	} else if flusher, ok := l.out.(interface{ Flush() error }); ok {
		_ = flusher.Flush()
	}

	// this generally shouldn't be necessary, but we could run into cases (e.g.
	// during a force-kill) where further Prints are called after a Close. These
	// should just do nothing.
	l.out = ioutil.Discard
	l.outBuf = bufio.NewWriter(l.out)
}

func (l *logger) println(line string) {

	l.l.Lock()
	defer l.l.Unlock()

	if l.timeFmt != "" {
		fmt.Fprintf(
			l.outBuf,
			"%s %c ",
			time.Now().Format(l.timeFmt),
			l.sep,
		)
	}

	fmt.Fprintf(
		l.outBuf,
		"%s%s%c %s\n",
		l.pname,
		strings.Repeat(" ", int(*l.maxPNameLen+1)-len(l.pname)),
		l.sep,
		line,
	)

	l.outBuf.Flush()
}

func (l *logger) Println(line string) {
	l.println(line)
}

func (l *logger) Printf(msg string, args ...interface{}) {
	l.Println(fmt.Sprintf(msg, args...))
}

type processConfig struct {
	pmuxproc.Config `yaml:",inline"`
	Name            string `yaml:"name"`
}

type config struct {
	TimeFormat string          `yaml:"timeFormat"`
	Processes  []processConfig `yaml:"processes"`
}

func (cfg config) init() (config, error) {

	if len(cfg.Processes) == 0 {
		return config{}, errors.New("no processes defined")
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

	stdoutLogger := newLogger(os.Stdout, logSepStdout, cfg.TimeFormat)
	defer stdoutLogger.Close()

	stderrLogger := newLogger(os.Stderr, logSepStderr, cfg.TimeFormat)
	defer stderrLogger.Close()

	sysLogger := stderrLogger.withSep(logSepSys)

	defer sysLogger.Println("exited gracefully, ciao!")

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		sigCh := make(chan os.Signal, 2)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

		sig := <-sigCh
		sysLogger.Printf("%v signal received, waiting for child processes to exit", sig)
		cancel()

		<-sigCh
		sysLogger.Printf("forcefully exiting pmux process, there may be zombie child processes being left behind, good luck!")
		sysLogger.Close()
		os.Exit(1)
	}()

	var wg sync.WaitGroup
	defer wg.Wait()

	for _, cfgProc := range cfg.Processes {
		wg.Add(1)
		go func(procCfg processConfig) {
			defer wg.Done()

			stdoutLogger := stdoutLogger.withPName(procCfg.Name)
			stderrLogger := stderrLogger.withPName(procCfg.Name)
			sysLogger := sysLogger.withPName(procCfg.Name)

			defer sysLogger.Printf("stopped process handler")

			sysLogger.Println("starting process")

			pmuxproc.RunProcess(
				ctx, stdoutLogger, stderrLogger, sysLogger, procCfg.Config,
			)
		}(cfgProc)
	}
}

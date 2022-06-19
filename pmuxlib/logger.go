package pmuxlib

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"sync"
	"time"
)

// pname used by pmux itself for logging.
const pmuxPName = "pmux"

// characters used to denote different kinds of logs
const (
	logSepStdout = '›'
	logSepStderr = '»'
	logSepSys    = '~'
)

// Logger is used by RunProcess to log process details in realtime. You can use
// a new(NullLogger) if you don't care.
type Logger interface {
	Println(string)
	Printf(string, ...interface{})
}

// NullLogger is an implementation of Logger which doesn't do anything.
type NullLogger struct{}

func (*NullLogger) Println(string)                {}
func (*NullLogger) Printf(string, ...interface{}) {}

// PlainLogger implements Logger by writing each line directly to the given
// io.Writer as-is.
type PlainLogger struct {
	io.Writer
}

func (l PlainLogger) Println(line string) {
	fmt.Fprintln(l, line)
}

func (l PlainLogger) Printf(str string, args ...interface{}) {
	fmt.Fprintf(l, str, args...)
}

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

package pmuxproc

import (
	"fmt"
	"io"
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

package pmuxproc

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

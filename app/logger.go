package app

import (
	"fmt"
	"log"
	"os"
)

type LogLevel int

const (
	LogError LogLevel = iota
	LogVerbose
	LogTrace
)

type Logger interface {
	Output(calldepth int, s string) error
}

// Logger is a struct for embedding std loggers
type logger struct {
	logLevel         LogLevel
	warn, log, trace Logger
}

// Tracef prints message to Stdout (l.trace variable).
func (l logger) Tracef(format string, v ...interface{}) {
	if l.trace != nil && l.logLevel >= LogTrace {
		l.trace.Output(2, fmt.Sprintf(format, v...))
	}
}

// Printf prints message to Stdout (l.log variable).
func (l logger) Printf(format string, v ...interface{}) {
	if l.log != nil && l.logLevel >= LogVerbose {
		l.log.Output(2, fmt.Sprintf(format, v...))
	}
}

// Errorf prints message to Stderr (l.warn variable an logLevel is set).
func (l logger) Errorf(format string, v ...interface{}) {
	if l.warn != nil && l.logLevel >= LogError {
		l.warn.Output(2, fmt.Sprintf(format, v...))
	}
}

// SetStdLoggers initializes trace,log,warn with std loggers.
func (l *logger) SetStdLoggers() {
	l.trace = log.New(os.Stdout, "T", log.LstdFlags|log.Lshortfile)
	l.log = log.New(os.Stdout, "D", log.LstdFlags|log.Lshortfile)
	l.warn = log.New(os.Stderr, "E", log.LstdFlags|log.Lshortfile)
}

// SetLoggers sets 3 std loggers.
func (l *logger) SetLoggers(warn, log, trace Logger) {
	l.warn, l.log, l.trace = warn, log, trace
}

// SetLogLevel sets minimum log level.
func (l *logger) SetLogLevel(level LogLevel) {
	l.logLevel = level
}

package trace

import (
	"fmt"
	"log"
	"os"
)

var (
	trace   = log.New(os.Stdout, "T", log.LstdFlags|log.Lshortfile)
	enabled bool
)

// Init is a function that enables trace output to Stdout.
func Init(enable bool) {
	enabled = enable
}

// Printf calls l.Output to print to the logger.
// Arguments are handled in the manner of fmt.Printf.
func Printf(format string, v ...interface{}) {
	if enabled {
		trace.Output(2, fmt.Sprintf(format, v...))
	}
}

// Print calls l.Output to print to the logger.
// Arguments are handled in the manner of fmt.Print.
func Print(v ...interface{}) {
	if enabled {
		trace.Output(2, fmt.Sprint(v...))
	}
}

// Println calls l.Output to print to the logger.
// Arguments are handled in the manner of fmt.Println.
func Println(v ...interface{}) {
	if enabled {
		trace.Output(2, fmt.Sprintln(v...))
	}
}

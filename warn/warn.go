package warn

import (
	"fmt"
	"github.com/semrush/ws2http/warn/trace"
	"io/ioutil"
	"log"
	"os"
)

var warn = log.New(os.Stderr, "E", log.LstdFlags|log.Lshortfile)

// init is a function that sets prefix(D), output(stdout) and file & line to std logger.
func init() {
	log.SetPrefix("D")
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

// Init is a function that discards std logger output if verbose or trace flag was not set.
func Init(withVerbose, withTrace bool) {
	if !withVerbose && !withTrace {
		log.SetOutput(ioutil.Discard)
	}

	trace.Init(withTrace)
}

// Printf calls l.Output to print to the logger.
// Arguments are handled in the manner of fmt.Printf.
func Printf(format string, v ...interface{}) { warn.Output(2, fmt.Sprintf(format, v...)) }

// Print calls l.Output to print to the logger.
// Arguments are handled in the manner of fmt.Print.
func Print(v ...interface{}) { warn.Output(2, fmt.Sprint(v...)) }

// Println calls l.Output to print to the logger.
// Arguments are handled in the manner of fmt.Println.
func Println(v ...interface{}) { warn.Output(2, fmt.Sprintln(v...)) }

// Fatal calls l.Output to print to the logger.
// Arguments are handled in the manner of fmt.Print.
func Fatal(v ...interface{}) { warn.Output(2, fmt.Sprint(v...)); os.Exit(1) }

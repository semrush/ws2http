package main

import (
	"flag"
	"fmt"
	"github.com/semrush/ws2http/app"
	"io/ioutil"
	"log"
	"os"
	"strings"
)

var Version string

const AppName = "ws2http"

var (
	flHost        = flag.String("h", "localhost:8090", "websocket listen address")
	flHeaders     = flag.String("headers", "Authorization", "allow set custom http headers to rpc backend via comma")
	flTimeout     = flag.Int("timeout", 20, "timeout in seconds for http requests")
	flMaxParallel = flag.Int("c", 10, "max parallel http requests per host")
	flVerbose     = flag.Bool("verbose", false, "enable debug output")
	flTrace       = flag.Bool("trace", false, "enable trace output")
	flRoutes      StringFlags

	flDst = flag.String("dst", "", "deprecated, use 'route' flag instead")     // deprecated, old syntax support
	flSrc = flag.String("src", "/rpc", "deprecated, use 'route' flag instead") // deprecated, old syntax support
)

func main() {
	flag.Var(&flRoutes, "route", "mapping from websocket endpoint to http endpoint, like /rpc:http://localhost/rpc")
	flag.Parse()
	fixStdLog(*flVerbose, *flTrace)

	if len(flRoutes.ProxyRules()) == 0 && (*flSrc == "" && *flDst == "") {
		flag.PrintDefaults()
		return
	}

	// support old syntax rules for -dst -src
	rules := flRoutes.ProxyRules()
	if *flSrc != "" && *flDst != "" {
		rules = append(rules, app.ProxyRule{Src: *flSrc, DstUrl: *flDst})
	}

	a := &app.App{
		AppName:             AppName,
		ListenAddr:          *flHost,
		RedirectRules:       rules,
		Headers:             strings.Split(*flHeaders, ","),
		Timeout:             *flTimeout,
		MaxParallelRequests: *flMaxParallel,
	}

	a.SetStdLoggers()
	a.SetLogLevel(logLevel(*flVerbose, *flTrace))
	a.Printf("starting %s version=%s", AppName, Version)
	if err := a.Run(); err != nil {
		log.SetOutput(os.Stderr)
		log.Fatal(err.Error())
	}
}

func logLevel(verbose, trace bool) app.LogLevel {
	if trace {
		return app.LogTrace
	} else if verbose {
		return app.LogVerbose
	}

	return app.LogError
}

// fixStdLog sets additional params to std logger (prefix D, filename & line).
func fixStdLog(verbose, trace bool) {
	log.SetPrefix("D")
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	if verbose || trace {
		log.SetOutput(os.Stdout)
	} else {
		log.SetOutput(ioutil.Discard)
	}
}

type StringFlags struct{ v []string }

func (f *StringFlags) String() string {
	return fmt.Sprint(f.v)
}

func (f *StringFlags) Set(value string) error {
	if strings.Count(value, ":") >= 2 {
		f.v = append(f.v, value)
		return nil
	}

	return fmt.Errorf("invalid syntax: %v", value)
}

func (f StringFlags) ProxyRules() []app.ProxyRule {
	pv := []app.ProxyRule{}
	for _, v := range f.v {
		routes := strings.SplitN(v, ":", 2)
		pv = append(pv, app.ProxyRule{Src: routes[0], DstUrl: routes[1]})
	}

	return pv
}

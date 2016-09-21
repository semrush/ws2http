package main

import (
	"flag"
	"fmt"
	"github.com/semrush/ws2http/app"
	"github.com/semrush/ws2http/warn"
	"log"
	"strings"
)

var Version string

var (
	flHost        = flag.String("h", "localhost:8090", "websocket listen address")
	flHeaders     = flag.String("headers", "Authorization", "allow set custom http headers to rpc backend via comma")
	flTimeout     = flag.Int("timeout", 20, "timeout in seconds for http requests")
	flMaxParallel = flag.Int("c", 10, "max parallel http requests per host")
	flVerbose     = flag.Bool("verbose", false, "enable debug output")
	flTrace       = flag.Bool("trace", false, "enable trace output")
	flRoutes      StringFlags
)

func main() {
	flag.Var(&flRoutes, "route", "mapping from websocket endpoint to http endpoint, like /rpc:http://localhost/rpc")
	flag.Parse()
	warn.Init(*flVerbose, *flTrace)
	if len(flRoutes.ProxyRules()) == 0 {
		flag.PrintDefaults()
		return
	}

	log.Printf("starting ws2http version=%s", Version)
	app := &app.App{
		ListenAddr:          *flHost,
		RedirectRules:       flRoutes.ProxyRules(),
		Headers:             strings.Split(*flHeaders, ","),
		Timeout:             *flTimeout,
		MaxParallelRequests: *flMaxParallel,
	}

	if err := app.Run(); err != nil {
		warn.Fatal(err.Error())
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
		pv = append(pv, app.ProxyRule{routes[0], routes[1]})
	}

	return pv
}

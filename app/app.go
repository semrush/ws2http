package app

import (
	"errors"
	"net/http"

	"golang.org/x/net/websocket"
)

type ProxyRule struct {
	Src, DstUrl string
}

type App struct {
	ListenAddr                   string
	RedirectRules                []ProxyRule
	Headers                      []string
	Timeout, MaxParallelRequests int

	logger
}

var ErrNoEndpoints = errors.New("no endpoints were defined")

// Run runs web server with specified redirect rules.
func (a *App) Run() error {
	if len(a.RedirectRules) == 0 {
		return ErrNoEndpoints
	}

	for _, r := range a.RedirectRules {
		a.Printf("adding rule from=ws://%s%s to=%s, allowed_headers=%s timeout=%ds parallel_requests=%d", a.ListenAddr, r.Src, r.DstUrl, a.Headers, a.Timeout, a.MaxParallelRequests)

		hf := NewHttpForwarder(r.DstUrl, a.Headers, a.Timeout, a.MaxParallelRequests)
		hf.SetLoggers(a.warn, a.log, a.trace)
		hf.SetLogLevel(a.logLevel)

		http.Handle(r.Src, websocket.Handler(hf.Handler))
	}

	return http.ListenAndServe(a.ListenAddr, nil)
}

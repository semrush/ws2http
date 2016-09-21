package app

import (
	"errors"
	"golang.org/x/net/websocket"
	"log"
	"net/http"
)

type ProxyRule struct {
	Src, DstUrl string
}

type App struct {
	ListenAddr                   string
	RedirectRules                []ProxyRule
	Headers                      []string
	Timeout, MaxParallelRequests int
}

func (a *App) Run() error {
	if len(a.RedirectRules) == 0 {
		return errors.New("no endpoints were defined")
	}

	for _, r := range a.RedirectRules {
		log.Printf("adding rule from=ws://%s%s to=%s, allowed_headers=%s timeout=%ds parallel_requests=%d", a.ListenAddr, r.Src, r.DstUrl, a.Headers, a.Timeout, a.MaxParallelRequests)
		http.Handle(r.Src, websocket.Handler(NewHttpForwarder(r.DstUrl, a.Headers, a.Timeout, a.MaxParallelRequests).Handler))
	}

	return http.ListenAndServe(a.ListenAddr, nil)
}

package app

import (
	"errors"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/net/websocket"
)

type ProxyRule struct {
	Src, DstUrl string
}

type App struct {
	AppName                      string
	ListenAddr                   string
	RedirectRules                []ProxyRule
	Headers                      []string
	Timeout, MaxParallelRequests int

	logger

	statBackendRequests  *prometheus.CounterVec
	statBackendDurations *prometheus.SummaryVec
	statActiveConns      *prometheus.GaugeVec
}

var ErrNoEndpoints = errors.New("no endpoints were defined")

// Run runs web server with specified redirect rules.
func (a *App) Run() error {
	if len(a.RedirectRules) == 0 {
		return ErrNoEndpoints
	}

	a.registerMetrics()

	// set redirect rules, handle specific endpoint
	for _, r := range a.RedirectRules {
		hf := a.newHttpForwarder(r.Src, r.DstUrl)
		http.Handle(r.Src, websocket.Handler(hf.Handler))
	}

	// handle all src:dstUrl endpoint in one / handler
	ghf := a.newHttpForwarder("/", "*", a.RedirectRules...)
	http.Handle("/", websocket.Handler(ghf.Handler))

	// start server
	a.Printf("starting http listener at http://%s\n", a.ListenAddr)
	return http.ListenAndServe(a.ListenAddr, nil)
}

func (a *App) newHttpForwarder(src, dstUrl string, rule ...ProxyRule) *HttpForwarder {
	a.Printf("adding rule from=ws://%s%s to=%s, allowed_headers=%s timeout=%ds parallel_requests=%d", a.ListenAddr, src, dstUrl, a.Headers, a.Timeout, a.MaxParallelRequests)

	hf := NewHttpForwarder(dstUrl, a.Headers, a.Timeout, a.MaxParallelRequests)
	hf.SetLoggers(a.warn, a.log, a.trace)
	hf.SetLogLevel(a.logLevel)
	hf.SetStats(a.statBackendRequests, a.statBackendDurations, a.statActiveConns)

	if len(rule) > 0 {
		hf.SetMultiMode(rule)
	}

	return hf
}

// registerMetrics is a function that initializes a.stat* variables and adds /metrics endpoint to echo.
func (a *App) registerMetrics() {
	a.statActiveConns = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: a.AppName,
		Subsystem: "ws",
		Name:      "connections_total",
		Help:      "Current active websocket connections by uri.",
	}, []string{"uri"})

	a.statBackendRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: a.AppName,
		Subsystem: "proxy",
		Name:      "requests_total",
		Help:      "Requests to backend by url/method/status.",
	}, []string{"url", "method", "status"}) //status: ok, timeout, error

	a.statBackendDurations = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace: a.AppName,
		Subsystem: "proxy",
		Name:      "rpc_duration_seconds",
		Help:      "Response time by rpc method/http status code.",
	}, []string{"url", "method", "code"}) // http code

	prometheus.MustRegister(a.statActiveConns, a.statBackendRequests, a.statBackendDurations)
	a.Printf("registering /metrics url as prometheus handler")
	http.Handle("/metrics", promhttp.Handler())
}

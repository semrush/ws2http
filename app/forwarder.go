package app

import (
	"bytes"
	"crypto/tls"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/net/websocket"
	"strconv"
)

const (
	maxConnectionToHost = 128
)

type errTimeout interface {
	Timeout() bool
}

// HttpForwarder for every incoming connection
type HttpForwarder struct {
	srcUrl, dstUrl               string
	allowedHeaders               []string
	timeout, maxParallelRequests int
	transport                    *http.Transport

	logger

	statBackendRequests  *prometheus.CounterVec
	statBackendDurations *prometheus.SummaryVec
	statActiveConns      *prometheus.GaugeVec
}

// NewHttpForwarder returns new single instance HttpForwarder for connection.
func NewHttpForwarder(dstUrl string, allowedHeaders []string, timeout, maxParallelRequests int) *HttpForwarder {
	return &HttpForwarder{
		dstUrl:              dstUrl,
		allowedHeaders:      allowedHeaders,
		timeout:             timeout,
		maxParallelRequests: maxParallelRequests,
		transport: &http.Transport{
			MaxIdleConnsPerHost: maxConnectionToHost,
			TLSClientConfig: &tls.Config{
				ClientSessionCache: tls.NewLRUClientSessionCache(maxConnectionToHost),
				InsecureSkipVerify: true,
			},
		},
	}
}

func (hf *HttpForwarder) SetStats(requests *prometheus.CounterVec, durations *prometheus.SummaryVec, conns *prometheus.GaugeVec) {
	hf.statBackendRequests = requests
	hf.statBackendDurations = durations
	hf.statActiveConns = conns
}

// isAllowedHeader is a function that checks existence of header in allowedHeaders
func (hf *HttpForwarder) isAllowedHeader(header string) bool {
	for _, h := range hf.allowedHeaders {
		if h == header {
			return true
		}
	}

	return false
}

// Handler is a handler function for handling connection from WS.
func (hf *HttpForwarder) Handler(ws *websocket.Conn) {
	// count active conns
	hf.srcUrl = ws.Request().URL.Path
	if hf.statActiveConns != nil {
		hf.statActiveConns.WithLabelValues(hf.srcUrl).Inc()
		defer hf.statActiveConns.WithLabelValues(hf.srcUrl).Dec()
	}

	// debug events
	debug.events <- debugMessage{msgType: clientConnected, req: ws.Request()}
	defer func() { debug.events <- debugMessage{msgType: clientDisconnected, req: ws.Request()} }()

	var (
		err                error
		msg                []byte
		client             = &http.Client{Timeout: time.Duration(hf.timeout) * time.Second, Transport: hf.transport}
		maxParallelRequest = make(chan struct{}, hf.maxParallelRequests)
		headers            = http.Header{}
	)

	for {
		if err = websocket.Message.Receive(ws, &msg); err != nil {
			if err != io.EOF {
				hf.Errorf("error while receiving data from client=%s err=%s data=%s", ws.Request().RemoteAddr, err, msg)
			}
			break
		}

		hf.Tracef("type=request ip=%s data=%s custom_header=%+v", ws.Request().RemoteAddr, msg, headers)
		debug.events <- debugMessage{msgType: wsRequest, req: ws.Request(), data: msg}

		// set custom headers for session
		if bytes.HasPrefix(msg, []byte("SET ")) {
			hv := strings.Split(string(msg[4:]), " ")
			if hf.isAllowedHeader(hv[0]) {
				headers.Set(hv[0], hv[1]) // TODO(sergeyfast): possible race condition while doPostRequest
			} else {
				hf.Printf("failed to add custom header=%v value=%v ip=%s", hv[0], hv[1], ws.Request().RemoteAddr)
			}

			continue
		}

		maxParallelRequest <- struct{}{}
		go func(msg []byte) {
			var resp []byte
			now := time.Now()
			rc, err, rpcErr := hf.doPostRequest(client, msg, headers)
			duration := time.Since(now)
			<-maxParallelRequest

			// save stat
			hf.statRequest(methodFromRequest(msg), duration, err, rpcErr)

			// process response
			if rpcErr != nil {
				// go
			} else if err != nil {
				if err != io.EOF {
					hf.Errorf("not eof err=%v", err)
				}
				return
			} else if resp, err = ioutil.ReadAll(rc); err != nil {
				hf.Errorf("read err=%v", err)
				rpcErr = NewJsonRpcErrResponse(msg, -200, err)
			}

			if rpcErr != nil {
				resp = rpcErr.ToJSON()
				hf.Errorf("rpc err=%v", rpcErr)
			}

			hf.Tracef("type=response ip=%s duration=%s data=%s", ws.Request().RemoteAddr, duration, resp)
			debug.events <- debugMessage{msgType: httpResponse, req: ws.Request(), data: resp}
			if err = websocket.Message.Send(ws, string(resp)); err != nil {
				hf.Errorf("can't send data to client=%s err=%s", ws.RemoteAddr().String(), err)
			}

			return
		}(msg)
	}
}

// statRequest logs requests durations.
func (hf *HttpForwarder) statRequest(method string, duration time.Duration, err error, rpcErr *JsonRpcErrResponse) {
	if hf.statBackendDurations == nil && hf.statBackendRequests == nil {
		return
	}

	status, httpCode := "ok", "200"
	if rpcErr != nil {
		status, httpCode = "error", strconv.Itoa(rpcErr.Error.Code)
	}

	if err != nil {
		if t, ok := err.(errTimeout); ok && t.Timeout() {
			status = "timeout"
		}
	}

	hf.statBackendRequests.WithLabelValues(hf.srcUrl, method, status).Inc()
	hf.statBackendDurations.WithLabelValues(hf.srcUrl, method, httpCode).Observe(duration.Seconds())
}

// doPostRequest sends http post request to json-rpc 2.0 endpoint.
func (hf *HttpForwarder) doPostRequest(client *http.Client, postData []byte, headers http.Header) (rc io.ReadCloser, err error, rpcErr *JsonRpcErrResponse) {
	var httpCode int
	req, err := http.NewRequest("POST", hf.dstUrl, bytes.NewBuffer(postData))
	defer func() {
		if err == nil && httpCode == http.StatusOK {
			return
		}

		rpcErr = NewJsonRpcErrResponse(postData, httpCode, err)
		return
	}()

	if err != nil {
		hf.Errorf("http new request err=%s", err)
		return
	}

	req.Header = headers
	resp, err := client.Do(req)
	if err != nil {
		hf.Errorf("client.Do() request failed url=%s err=%s data=%s", hf.dstUrl, err, postData)
		return
	}

	httpCode = resp.StatusCode
	rc = resp.Body

	return
}

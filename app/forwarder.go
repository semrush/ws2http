package app

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/net/websocket"
)

const (
	maxConnectionToHost = 128
)

var errInvalidPrefix = errors.New("invalid prefix: dstUrl was not found")

type errTimeout interface {
	Timeout() bool
}

type rpcRequest struct {
	req    JsonRpcRequest // rewrited request
	srcUrl string         // source handler, like / or /rpc
	dstUrl string         // json-rpc server endpoint
	msg    []byte         // rewrited msg
}

// JSON marshals rpcRequest ignoring errors.
func (r rpcRequest) JSON() []byte {
	data, err := json.Marshal(r.req)
	if err != nil {
		log.Println(err)
	}

	return data
}

// requestForwarder is a struct for handling every client connection and request.
type requestForwarder struct {
	client             *http.Client
	maxParallelRequest chan struct{}
	headers            http.Header
	headersLock        *sync.RWMutex
	allowedHeaders     []string
	multipleRules      map[string]ProxyRule // special multiple rules mode
	ws                 *websocket.Conn

	logger
}

// newRequestForwarder returns new request forwarder with predefined http.Client and logger from HTTP Forwarder.
func (hf *HttpForwarder) newRequestForwarder(ws *websocket.Conn) requestForwarder {
	rf := requestForwarder{
		client: &http.Client{
			Timeout:   time.Duration(hf.timeout) * time.Second,
			Transport: hf.transport,
		},
		maxParallelRequest: make(chan struct{}, hf.maxParallelRequests),
		headers:            make(http.Header),
		ws:                 ws,
		allowedHeaders:     hf.allowedHeaders,
		multipleRules:      hf.multipleRules,
		headersLock:        &sync.RWMutex{},
	}

	rf.SetLogLevel(hf.logLevel)
	rf.SetLoggers(hf.warn, hf.log, hf.trace)

	return rf
}

// isAllowedHeader is a function that checks existence of header in allowedHeaders
func (rf *requestForwarder) isAllowedHeader(header string) bool {
	for _, h := range rf.allowedHeaders {
		if h == header {
			return true
		}
	}

	return false
}

// checkAndSetHeaders checks message for SET prefix. If message contains header then set it and return true.
func (rf *requestForwarder) checkAndSetHeaders(msg []byte) bool {
	// TODO(sergeyfast): deprecated, remove before merging into master, check \n problem?
	if bytes.HasPrefix(msg, []byte("AUTH ")) {
		if rf.isAllowedHeader("Authorization") {
			rf.headersLock.Lock()
			defer rf.headersLock.Unlock()
			rf.headers.Set("Authorization", string(msg[5:]))
		}

		return true
	}

	// set custom headers for session
	if bytes.HasPrefix(msg, []byte("SET ")) {
		hv := strings.Split(string(msg[4:]), " ")
		if rf.isAllowedHeader(hv[0]) {
			rf.headersLock.Lock()
			defer rf.headersLock.Unlock()
			rf.headers.Set(hv[0], hv[1])
		} else {
			rf.Printf("failed to add custom header=%v value=%v ip=%s", hv[0], hv[1], rf.ws.Request().RemoteAddr)
		}

		return true
	}

	return false
}

// copyHeaders returns new copy from rf.headers.
func (rf *requestForwarder) copyHeaders() http.Header {
	rf.headersLock.RLock()
	defer rf.headersLock.RUnlock()

	locHeaders := make(http.Header)
	for k, vv := range rf.headers {
		for _, v := range vv {
			locHeaders.Add(k, v)
		}
	}

	return locHeaders
}

// rewriteRequest returns rpcRequest with src/dst urls, method and  error depends on msg prefix.
// Errors could be: unmarshal request, method not found, invalid prefix for routing.
// TODO(sergeyfast): add batch support
func (rf *requestForwarder) rewriteRequest(msg []byte, defaultDstUrl string) (rpcReq rpcRequest, err error) {
	var req JsonRpcRequest
	if err = json.Unmarshal(msg, &req); err != nil {
		return // invalid json-rpc request
	}

	srcUrl := "/"
	if rf.ws.Request() != nil { // could be nil while testing
		srcUrl = rf.ws.Request().URL.Path
	}

	rpcReq = rpcRequest{
		req:    req,
		msg:    msg,
		srcUrl: srcUrl,
	}

	// check for current requestForwarder mode: normal method without routing prefix
	if len(rf.multipleRules) == 0 {
		rpcReq.dstUrl = defaultDstUrl
		return
	}

	// rf has multiple routing: detect dstUrl from method prefix
	m := strings.SplitN(req.Method, ".", 2)
	if len(m) == 1 {
		err = errMethodFormat
		return
	} else {
		rpcReq.srcUrl = "/" + m[0]
	}

	// detect dstUrl by srcUrl
	if r, ok := rf.multipleRules[rpcReq.srcUrl]; !ok {
		err = errInvalidPrefix
		return
	} else {
		rpcReq.dstUrl = r.DstUrl
		rpcReq.req.Method = m[1]
		rpcReq.msg = rpcReq.JSON()
	}

	return
}

// HttpForwarder is a struct for unique endpoint.
type HttpForwarder struct {
	dstUrl                       string
	allowedHeaders               []string
	timeout, maxParallelRequests int
	transport                    *http.Transport

	multipleRules map[string]ProxyRule // special multiple rules mode

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

// SetMultiMode handles incoming requests and routes it into dstUrl by "src" prefix in method.
// For example:
// 	src = /rpc; dstUrl = http://localhost/rpc-service
//  rpc method = rpc.test.method
//  result: method = test.method, dstUrl = http://localhost/rpc-service [trimmed / in src].
func (hf *HttpForwarder) SetMultiMode(rules []ProxyRule) {
	hf.multipleRules = make(map[string]ProxyRule)
	for _, r := range rules {
		hf.multipleRules[r.Src] = r
	}
}

// Handler is a handler function for handling connection from WS.
func (hf *HttpForwarder) Handler(ws *websocket.Conn) {
	// todo check input url

	// count active conns for srcUrl
	if hf.statActiveConns != nil {
		hf.statActiveConns.WithLabelValues(ws.Request().URL.Path).Inc()
		defer hf.statActiveConns.WithLabelValues(ws.Request().URL.Path).Dec()
	}

	// send debug events
	debug.events <- debugMessage{msgType: clientConnected, req: ws.Request()}
	defer func() { debug.events <- debugMessage{msgType: clientDisconnected, req: ws.Request()} }()

	var (
		msg []byte                       // incoming WS message
		err error                        // last error
		rf  = hf.newRequestForwarder(ws) // forwarder per connection for handling custom headers, max parallel requests
	)

	for {
		// read incoming messages
		if err = websocket.Message.Receive(ws, &msg); err != nil {
			if err != io.EOF {
				hf.Errorf("error while receiving data from client=%s err=%s data=%s", ws.Request().RemoteAddr, err, msg)
			}
			break
		}

		hf.Tracef("type=request ip=%s data=%s custom_header=%+v", ws.Request().RemoteAddr, msg, rf.headers)
		debug.events <- debugMessage{msgType: wsRequest, req: ws.Request(), data: msg}

		// check for SET prefix and set headers if needed
		if rf.checkAndSetHeaders(msg) {
			continue
		}

		// check for multiple mode and rewrite message if needed
		rpcReq, err := rf.rewriteRequest(msg, hf.dstUrl)
		if err != nil {
			hf.Errorf("error while rewriting msg from client=%s err=%s data=%s", ws.Request().RemoteAddr, err, msg)
			if rpcReq.req.Id != nil {
				websocket.Message.Send(ws, string(NewJsonRpcErr(rpcReq.req, JsonRpcMethodNotFound, err).JSON()))
			}
			continue
		}

		// perform http request to backend
		rf.maxParallelRequest <- struct{}{}
		go func(rpcReq rpcRequest, headers http.Header) {
			var resp []byte
			now := time.Now()

			// do post request
			rc, err, rpcErr := hf.doPostRequest(rf.client, rpcReq.msg, rpcReq.dstUrl, headers)
			duration := time.Since(now)
			<-rf.maxParallelRequest

			// save stat
			hf.statRequest(rpcReq.srcUrl, rpcReq.req.Method, duration, err, rpcErr)

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
				rpcErr = NewJsonRpcErr(rpcReq.req, 200, err)
			}

			if rpcErr != nil {
				resp = rpcErr.JSON()
				hf.Errorf("rpc err=%v", rpcErr)
			}

			// trace events
			hf.Tracef("type=response ip=%s duration=%s data=%s", ws.Request().RemoteAddr, duration, resp)
			debug.events <- debugMessage{msgType: httpResponse, req: ws.Request(), data: resp}

			// send response
			if err = websocket.Message.Send(ws, string(resp)); err != nil {
				hf.Errorf("can't send data to client=%s lastErr=%s", ws.RemoteAddr().String(), err)
			}

			return
		}(rpcReq, rf.copyHeaders())
	}
}

// statRequest logs requests durations.
func (hf *HttpForwarder) statRequest(srcUrl, method string, duration time.Duration, err error, rpcErr *JsonRpcErrResponse) {
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

	hf.statBackendRequests.WithLabelValues(srcUrl, method, status).Inc()
	hf.statBackendDurations.WithLabelValues(srcUrl, method, httpCode).Observe(duration.Seconds())
}

// doPostRequest sends http post request to json-rpc 2.0 endpoint.
func (hf *HttpForwarder) doPostRequest(client *http.Client, postData []byte, dstUrl string, headers http.Header) (rc io.ReadCloser, err error, rpcErr *JsonRpcErrResponse) {
	var httpCode int
	req, err := http.NewRequest("POST", dstUrl, bytes.NewBuffer(postData))
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
	req.Header.Add("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		hf.Errorf("client.Do() request failed url=%s err=%s data=%s", dstUrl, err, postData)
		return
	}

	httpCode = resp.StatusCode
	rc = resp.Body

	return
}

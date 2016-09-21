package app

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"github.com/semrush/ws2http/warn"
	"github.com/semrush/ws2http/warn/trace"
	"golang.org/x/net/websocket"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	maxConnectionToHost = 128
)

// HttpForwarder for every incoming connection
type HttpForwarder struct {
	dstUrl                       string
	allowedHeaders               []string
	timeout, maxParallelRequests int
	customHeaders                map[string]string
	lockH                        sync.RWMutex // guards customHeaders
	transport                    *http.Transport
}

// NewHttpForwarder returns new single instance HttpForwarder for connection.
func NewHttpForwarder(dstUrl string, allowedHeaders []string, timeout, maxParallelRequests int) *HttpForwarder {
	return &HttpForwarder{
		dstUrl:              dstUrl,
		allowedHeaders:      allowedHeaders,
		customHeaders:       make(map[string]string),
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

// isAllowedHeader is a function that checks existence of header in allowedHeaders
func (hf *HttpForwarder) isAllowedHeader(header string) bool {
	for _, h := range hf.allowedHeaders {
		if h == header {
			return true
		}
	}

	return false
}

// addCustomHeader adds header+value to customHeaders map with lock.
func (hf *HttpForwarder) addCustomHeader(header, value string) {
	hf.lockH.Lock()
	defer hf.lockH.Unlock()
	hf.customHeaders[header] = value
}

// Handler is a handler function for handling connection from WS.
func (hf *HttpForwarder) Handler(ws *websocket.Conn) {
	var (
		err                error
		msg                []byte
		client             = &http.Client{Timeout: time.Duration(hf.timeout) * time.Second, Transport: hf.transport}
		maxParallelRequest = make(chan struct{}, hf.maxParallelRequests)
	)

	for {
		if err = websocket.Message.Receive(ws, &msg); err != nil {
			if err != io.EOF {
				warn.Printf("error while receiving data from client=%s err=%s data=%s", ws.Request().RemoteAddr, err, msg)
			}
			break
		}

		trace.Printf("type=reqeust ip=%s data=%s custom_header=%+v", ws.Request().RemoteAddr, msg, hf.customHeaders)

		// set custom headers for session
		if bytes.HasPrefix(msg, []byte("SET ")) {
			hv := strings.Split(string(msg[4:]), " ")
			if hf.isAllowedHeader(hv[0]) {
				hf.addCustomHeader(hv[0], hv[1])
			} else {
				trace.Printf("failed to add custom header=%v value=%v ip=%s", hv[0], hv[1], ws.Request().RemoteAddr)
			}

			continue
		}

		maxParallelRequest <- struct{}{}
		go func(msg []byte) {
			var resp []byte
			now := time.Now()
			rc, err, rpcErr := hf.doHttpRequest(client, msg)
			<-maxParallelRequest

			if rpcErr != nil {
				// go
			} else if err != nil {
				if err != io.EOF {
					warn.Print(err)
				}
				return
			} else if resp, err = ioutil.ReadAll(rc); err != nil {
				warn.Println(err)
				rpcErr = NewJsonRpcErrResponse(msg, -200, err)
			}

			if rpcErr != nil {
				resp = rpcErr.ToJSON()
				warn.Println(rpcErr)
			}

			trace.Printf("type=response ip=%s duration=%s data=%s", ws.Request().RemoteAddr, time.Since(now), resp)
			if err = websocket.Message.Send(ws, string(resp)); err != nil {
				warn.Printf("can't send data to client=%s err=%s", ws.RemoteAddr().String(), err)
			}

			return
		}(msg)
	}
}

// doHttpRequest sends http request to json-rpc 2.0 endpoint.
func (hf *HttpForwarder) doHttpRequest(client *http.Client, postData []byte) (rc io.ReadCloser, err error, rpcErr *JsonRpcErrResponse) {
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
		warn.Println(err)
		return
	}

	req.Header.Add("Accept-Encoding", "gzip")
	if len(hf.customHeaders) > 0 {
		hf.lockH.RLock()
		for h, v := range hf.customHeaders {
			req.Header.Add(h, v)
		}
		hf.lockH.RUnlock()
	}

	resp, err := client.Do(req)
	if err != nil {
		warn.Printf("client.Do() request failed url=%s err=%s data=%s", hf.dstUrl, err, postData)
		return
	}

	httpCode = resp.StatusCode
	// Check that the server actual sent compressed data

	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		rc, err = gzip.NewReader(resp.Body)
		if err != nil {
			return
		}
	default:
		rc = resp.Body
	}

	return
}

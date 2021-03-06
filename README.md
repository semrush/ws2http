ws2http 
======

JSON-RPC 2.0 WebSocket to HTTP proxy.

Requirements
------
  
  * Golang 1.5+ 

Usage
------

    Usage of ./ws2http:
      -c int
            max parallel http requests per host (default 10)
      -h string
            websocket listen address (default "localhost:8090")
      -headers string
            allow set custom http headers to rpc backend via comma (default "Authorization")
      -route value
            mapping from websocket endpoint to http endpoint, like /rpc:http://localhost/rpc (default [])
      -timeout int
            timeout in seconds for http requests (default 20)
      -trace
            enable trace output
      -verbose
            enable debug output



Features
------
 
 * Proxies all data from WS to HTTP endpoint
 * Timeout for http requests (default 20)
 * Concurrent http requests to host by session (default 10)
 * Trace logs (requests/responses)
 * Encapsulated http backend errors to JSON-RPC errors (returns -1 * httpStatusCode as error code)
 * Supports multiple endpoints
 * Supports /metrics endpoint as Prometheus handler
 * Supports /debug/conns endpoint as remote connection tracer
 
### Goals

 * [x] shared ws endpoint with internal routing with prefix for methods
 * [ ] better interface for /debug/conns
 * [ ] support batch requests for metrics and routing
 * [ ] rewrite HttpForwarder.Handler into smaller parts
 * [ ] expand batch requests


How to run
------
    go get github.com/semrush/ws2http
    $GOPATH/bin/ws2http -verbose -route /rpc:http://localhost/rpc/
   
### Examples
    
    var w = new WebSocket("ws://localhost/rpc"); w.onmessage = function(data) { console.log(data); };
    w.send('SET Authorization authValue')
    w.send('{"jsonrpc":"2.0","method":"Ping","id":"1"}')
    
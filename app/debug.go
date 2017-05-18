package app

import (
	"golang.org/x/net/websocket"
	"html/template"
	"log"
	"net/http"
)

type debugMessageType int

const (
	clientConnected debugMessageType = iota
	clientDisconnected
	wsRequest
	httpResponse

	eventsBuffer = 1000
)

type (
	clientConns map[string]*http.Request

	debugMessage struct {
		msgType debugMessageType
		req     *http.Request
		data    []byte
		//addr    string //?
	}

	debugApp struct {
		events chan debugMessage
		tasks  chan func(clientConns)
	}
)

var debug = debugApp{
	events: make(chan debugMessage, eventsBuffer),
	tasks:  make(chan func(clientConns), eventsBuffer),
}

func init() {
	http.HandleFunc("/debug/conns/", debug.index)
	http.HandleFunc("/debug/conns/trace", debug.trace)
	http.Handle("/debug/conns/ws", websocket.Handler(debug.wsHandler))
	go debug.run()
}

func (debugApp) run() {
	conns := make(clientConns)

	for {
		select {
		case e := <-debug.events:
			switch e.msgType {
			case clientConnected:
				conns[e.req.RemoteAddr] = e.req
			case clientDisconnected:
				delete(conns, e.req.RemoteAddr)
			case wsRequest:
			case httpResponse:
			}
		case f := <-debug.tasks:
			f(conns)
		}
	}
}

// index shows active connections to proxy.
func (d debugApp) index(w http.ResponseWriter, r *http.Request) {
	type addr struct {
		Addr, Referrer, UserAgent string
	}

	addrs, list := make(chan []addr), []addr{}
	d.tasks <- func(m clientConns) {
		for k, c := range m {
			list = append(list, addr{Addr: k, Referrer: c.Referer(), UserAgent: c.UserAgent()})
		}
		addrs <- list
	}

	var data struct {
		Len  int
		List []addr
	}

	data.List = <-addrs
	data.Len = len(data.List)

	if err := indexTmpl.Execute(w, data); err != nil {
		log.Print(err)
	}
}

var indexTmpl = template.Must(template.New("index").Parse(`<html><head>
<title>/debug/conns/</title>
</head>
<body>
<p>active connections: {{.Len}}
<table>
{{range .List}}
<tr><td><a href="trace?addr={{.Addr}}">{{.Addr}}</a></td><td>{{.UserAgent}}</td><td>{{.Referrer}}</td></tr>
{{end}}
</table>
<br></body></html>
`))

func (d debugApp) trace(w http.ResponseWriter, r *http.Request) {
	addr := r.FormValue("addr")

	connected := make(chan bool)
	d.tasks <- func(m clientConns) {
		_, ok := m[addr]
		connected <- ok
	}

	var data struct {
		Addr      string
		Connected bool
	}

	data.Addr = addr
	data.Connected = <-connected

	if err := traceTmpl.Execute(w, data); err != nil {
		log.Print(err)
	}
}

var traceTmpl = template.Must(template.New("trace").Parse(`<html><head>
<title>/debug/conns/trace</title>
</head>
<body>
<p><a href="/debug/conns/">back to list</a></p>
<strong>Addr: {{.Addr}}</strong>
{{if .Connected}}
<script>
	var w = new WebSocket("ws://localhost:8090/rpc"); w.onmessage = function(data) { console.log(data); };
	function (data) {
		console.log(data);
	}
</script>

<p id="data">

</p>
{{else}}
client disconnected
{{end}}
<br></body></html>
`))

func (debugApp) wsHandler(ws *websocket.Conn) {

}

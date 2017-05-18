package app

import (
	"golang.org/x/net/websocket"
	"html/template"
	"io"
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
	clientConns  map[string]*http.Request
	watcherConns map[string]map[string]traceRequest // target -> watchers -> watcher chan

	debugMessage struct {
		msgType debugMessageType
		req     *http.Request
		data    []byte
	}

	debugApp struct {
		events        chan debugMessage
		tasks         chan func(clientConns)
		traceRequests chan traceRequest
	}

	traceRequest struct {
		Addr       string
		TargetAddr string
		Msg        chan debugMessage
		Cancel     bool
	}
)

var debug = debugApp{
	events:        make(chan debugMessage, eventsBuffer),
	tasks:         make(chan func(clientConns), eventsBuffer),
	traceRequests: make(chan traceRequest, eventsBuffer),
}

func init() {
	http.HandleFunc("/debug/conns/", debug.index)
	http.HandleFunc("/debug/conns/trace", debug.trace)
	http.Handle("/debug/conns/ws", websocket.Handler(debug.wsHandler))
	go debug.run()
}

func (d debugApp) run() {
	sessions, watchers := make(clientConns), make(watcherConns)

	for {
		select {
		case e := <-d.events:
			switch e.msgType {
			case clientConnected:
				sessions[e.req.RemoteAddr] = e.req
			case clientDisconnected:
				delete(sessions, e.req.RemoteAddr)

				// close watchers
				for _, l := range watchers[e.req.RemoteAddr] {
					close(l.Msg)
				}
				delete(watchers, e.req.RemoteAddr)
			case wsRequest, httpResponse:
				for _, watcher := range watchers[e.req.RemoteAddr] {
					watcher.Msg <- e
				}
			}
		case tr := <-d.traceRequests:
			if tr.Cancel {
				delete(watchers[tr.TargetAddr], tr.Addr)
			} else {
				if _, ok := watchers[tr.TargetAddr]; !ok {
					watchers[tr.TargetAddr] = make(map[string]traceRequest)
				}

				watchers[tr.TargetAddr][tr.Addr] = tr
			}
		case f := <-d.tasks:
			f(sessions)
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
		Server    string
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
	var w = new WebSocket("ws://" + document.location.host + "/debug/conns/ws?addr={{.Addr}}"); w.onmessage = function(data) {
	    var tr = document.createElement("tr");
	    tr.innerHTML = "<td>" + data.timeStamp + "</td>";
	    var td = document.createElement("td");
	    td.innerText = data.data;
	    tr.appendChild(td);

		document.getElementById("output").appendChild(tr);
	};
</script>

<table><tbody id="output"></tbody></table>
{{else}}
client disconnected
{{end}}
<br></body></html>
`))

func (d debugApp) wsHandler(ws *websocket.Conn) {
	addr := ws.Request().FormValue("addr")
	info := make(chan debugMessage, eventsBuffer)

	// register & deregister user
	d.traceRequests <- traceRequest{Addr: ws.Request().RemoteAddr, TargetAddr: addr, Msg: info}
	defer func() { d.traceRequests <- traceRequest{Addr: ws.Request().RemoteAddr, TargetAddr: addr, Cancel: true} }()

	for m := range info {
		if err := websocket.Message.Send(ws, string(m.data)); err != nil {
			if err != io.EOF {
				log.Println(err)
			}

			return
		}
	}
}

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
	clientConns map[string]*http.Request
	traceConns  map[string]map[string]traceRequest // target -> tracers -> trace chan

	debugMessage struct {
		msgType debugMessageType
		req     *http.Request
		data    []byte
	}

	debugApp struct {
		events        chan debugMessage
		ops           chan func(clientConns)
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
	ops:           make(chan func(clientConns), eventsBuffer),
	traceRequests: make(chan traceRequest, eventsBuffer),
}

func init() {
	http.HandleFunc("/debug/conns/", debug.index)
	http.HandleFunc("/debug/conns/trace", debug.trace)
	http.Handle("/debug/conns/ws", websocket.Handler(debug.wsHandler))
	go debug.loop()
}

func (d debugApp) loop() {
	sessions, tracers := make(clientConns), make(traceConns)

	for {
		select {
		case e := <-d.events:
			switch e.msgType {
			case clientConnected:
				sessions[e.req.RemoteAddr] = e.req
			case clientDisconnected:
				delete(sessions, e.req.RemoteAddr)

				// close tracers
				for _, l := range tracers[e.req.RemoteAddr] {
					close(l.Msg)
				}
				delete(tracers, e.req.RemoteAddr)
			case wsRequest, httpResponse:
				for _, tracer := range tracers[e.req.RemoteAddr] {
					tracer.Msg <- e
				}
			}
		case tr := <-d.traceRequests:
			if tr.Cancel {
				delete(tracers[tr.TargetAddr], tr.Addr)
			} else {
				if _, ok := tracers[tr.TargetAddr]; !ok {
					tracers[tr.TargetAddr] = make(map[string]traceRequest)
				}

				tracers[tr.TargetAddr][tr.Addr] = tr
			}
		case op := <-d.ops:
			op(sessions)
		}
	}
}

// index shows active connections to proxy.
func (d debugApp) index(w http.ResponseWriter, r *http.Request) {
	type session struct {
		Addr, Referrer, UserAgent string
	}

	sessions := make(chan []session)

	// get sessions from main "loop"
	d.ops <- func(m clientConns) {
		var list []session
		for k, c := range m {
			list = append(list, session{Addr: k, Referrer: c.Referer(), UserAgent: c.UserAgent()})
		}
		sessions <- list
	}

	// fetch and render result
	tmpl := struct {
		Len  int
		List []session
	}{List: <-sessions}

	tmpl.Len = len(tmpl.List)
	if err := indexTmpl.Execute(w, tmpl); err != nil {
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

	// check if requested session exists
	connected := make(chan bool)
	d.ops <- func(m clientConns) {
		_, ok := m[addr]
		connected <- ok
	}

	tmpl := struct {
		Server    string
		Addr      string
		Connected bool
	}{Connected: <-connected, Addr: addr}

	if err := traceTmpl.Execute(w, tmpl); err != nil {
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

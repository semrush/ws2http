package app

import (
	"golang.org/x/net/websocket"
	"testing"
)

func TestRequestForwarderRewrite(t *testing.T) {
	var tc = []struct {
		in, out     []byte
		m, src, dst string
		err         error
	}{
		{
			in:  []byte(`{"jsonrpc":"2.0","method":"test.subtract","params":[42,23],"id":1}`),
			out: []byte(`{"jsonrpc":"2.0","id":1,"method":"subtract","params":[42,23]}`),
			src: "/test", m: "subtract", dst: "http://test",
		},
		{
			in:  []byte(`{"jsonrpc":"2.0","method":"subtract","params":[42,23],"id":1}`),
			out: []byte(`{"jsonrpc":"2.0","method":"subtract","params":[42,23],"id":1}`),
			src: "/", m: "subtract", err: errMethodFormat,
		},
		{
			in:  []byte(`{"jsonrpc":"2.0","method":"rpc.test.subtract","params":[42,23],"id":1}`),
			out: []byte(`{"jsonrpc":"2.0","id":1,"method":"test.subtract","params":[42,23]}`),
			src: "/rpc", m: "test.subtract", dst: "http://rpc",
		},
		{
			in:  []byte(`{"jsonrpc":"2.0","method":"rpc1.test.subtract","params":[42,23],"id":1}`),
			out: []byte(`{"jsonrpc":"2.0","method":"rpc1.test.subtract","params":[42,23],"id":1}`),
			src: "/rpc1", m: "rpc1.test.subtract", err: errInvalidPrefix,
		},
		{
			in:  []byte(`{}`),
			out: []byte(`{}`),
			src: "/", m: "", err: errMethodFormat,
		},
	}

	hf := NewHttpForwarder("/", nil, 0, 0)
	hf.SetMultiMode(
		[]ProxyRule{
			{"/rpc", "http://rpc"},
			{"/test", "http://test"},
		},
	)
	rf := hf.newRequestForwarder(&websocket.Conn{})

	for _, c := range tc {
		rpcReq, err := rf.rewriteRequest(c.in, hf.dstUrl)
		if rpcReq.srcUrl != c.src || rpcReq.req.Method != c.m || string(c.out) != string(rpcReq.msg) {
			t.Errorf("rewrite(%s): got = %v, %v, %v, %v; expected = %v, %v,  %v, %v", string(c.in), rpcReq.srcUrl, rpcReq.req.Method, string(rpcReq.msg), err, c.src, c.m, string(c.out), c.err)
		}
	}
}

func TestRequestForwarderNoRewrite(t *testing.T) {
	var tc = []struct {
		in, out     []byte
		m, src, dst string
		err         error
	}{
		{
			in:  []byte(`{"jsonrpc":"2.0","method":"test.subtract","params":[42,23],"id":1}`),
			out: []byte(`{"jsonrpc":"2.0","method":"test.subtract","params":[42,23],"id":1}`),
			src: "/", m: "test.subtract",
		},
		{
			in:  []byte(`{"jsonrpc":"2.0","method":"subtract","params":[42,23],"id":1}`),
			out: []byte(`{"jsonrpc":"2.0","method":"subtract","params":[42,23],"id":1}`),
			src: "/", m: "subtract",
		},
		{
			in:  []byte(`{"jsonrpc":"2.0","method":"rpc.test.subtract","params":[42,23],"id":1}`),
			out: []byte(`{"jsonrpc":"2.0","method":"rpc.test.subtract","params":[42,23],"id":1}`),
			src: "/", m: "rpc.test.subtract",
		},
		{
			in:  []byte(`{"jsonrpc":"2.0","method":"rpc1.test.subtract","params":[42,23],"id":1}`),
			out: []byte(`{"jsonrpc":"2.0","method":"rpc1.test.subtract","params":[42,23],"id":1}`),
			src: "/", m: "rpc1.test.subtract",
		},
		{
			in:  []byte(`{}`),
			out: []byte(`{}`),
			src: "/", m: "",
		},
	}

	hf := NewHttpForwarder("/", nil, 0, 0)
	rf := hf.newRequestForwarder(&websocket.Conn{})

	for _, c := range tc {
		rpcReq, err := rf.rewriteRequest(c.in, hf.dstUrl)
		if rpcReq.srcUrl != c.src || rpcReq.req.Method != c.m || string(c.out) != string(rpcReq.msg) {
			t.Errorf("rewrite(%s): got = %v, %v, %v, %v; expected = %v, %v,  %v, %v", string(c.in), rpcReq.srcUrl, rpcReq.req.Method, string(rpcReq.msg), err, c.src, c.m, string(c.out), c.err)
		}
	}
}

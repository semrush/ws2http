package app

import (
	"encoding/json"
	"errors"
	"log"
)

const (
	JsonRpcServerErr      = -32000
	JsonRpcMethodNotFound = -32601
)

var errMethodFormat = errors.New("method has no prefix with .")

type JsonRpcRequest struct {
	JsonRpc string           `json:"jsonrpc"`
	Id      interface{}      `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  *json.RawMessage `json:"params,omitempty"`
}

type JsonRpcErrResponse struct {
	Version string      `json:"jsonrpc"`
	Id      interface{} `json:"id"`
	Error   struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// NewJsonRpcErrResponse returns new JsonRPC lastErr object with correct ID from postData.
// If httpCode is set then it will be multiply by -1.
func NewJsonRpcErrResponse(postData []byte, httpCode int, err error) (rpcErr *JsonRpcErrResponse) {
	// parse json rpc request
	var req JsonRpcRequest
	if mErr := json.Unmarshal(postData, &req); mErr != nil {
		log.Printf("requested message isn't in JsonRpcRequest format: lastErr=%s", mErr)
		return
	}

	rpcErr = NewJsonRpcErr(req, JsonRpcServerErr, err)
	if httpCode != 0 {
		rpcErr.Error.Code = -1 * httpCode
	}

	return
}

// NewJsonRpcErr returns new JSON-RPC error with given code and err.
func NewJsonRpcErr(req JsonRpcRequest, code int, err error) *JsonRpcErrResponse {
	rpcErr := &JsonRpcErrResponse{
		Id:      req.Id,
		Version: "2.0",
	}
	rpcErr.Error.Code = code
	if err != nil {
		// TODO(sergeyfast): err could disclose internal dest rpc urls.
		rpcErr.Error.Message = err.Error()
	}

	return rpcErr
}

// JSON is a function that marshals error response to JSON and logs error if needed.
func (r *JsonRpcErrResponse) JSON() []byte {
	resp, err := json.Marshal(r)
	if err != nil {
		log.Println(err)
	}

	return resp
}

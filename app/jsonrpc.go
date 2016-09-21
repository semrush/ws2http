package app

import (
	"encoding/json"
	"github.com/semrush/ws2http/warn"
)

const JsonRpcServerErr = -32000

type JsonRpcRequest struct {
	Id interface{} `json:"id"`
}

type JsonRpcErrResponse struct {
	Version string      `json:"jsonrpc"`
	Id      interface{} `json:"id"`
	Error   struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// NewJsonRpcErrResponse returns new JsonRPC err object with correct ID from postData.
// If httpCode is set then it will be multiply by -1.
func NewJsonRpcErrResponse(postData []byte, httpCode int, err error) (rpcErr *JsonRpcErrResponse) {
	// parse json rpc request
	var req JsonRpcRequest
	if mErr := json.Unmarshal(postData, &req); mErr != nil {
		warn.Printf("requested message isn't in JsonRpcRequest format: err=%s", mErr)
		return
	}

	rpcErr = &JsonRpcErrResponse{
		Id:      req.Id,
		Version: "2.0",
	}

	if err != nil {
		rpcErr.Error.Message = err.Error()
	}

	if httpCode == 0 {
		rpcErr.Error.Code = JsonRpcServerErr
	} else {
		rpcErr.Error.Code = -1 * httpCode
	}

	return
}

// ToJSON is a function that marshals error response to JSON and logs error if needed.
func (r *JsonRpcErrResponse) ToJSON() []byte {
	resp, err := json.Marshal(r)
	if err != nil {
		warn.Println(err)
	}

	return resp
}

package app

import (
	"encoding/json"
	"log"
)

const JsonRpcServerErr = -32000

type JsonRpcRequest struct {
	Id     interface{} `json:"id"`
	Method string      `json:"method"`
}

type JsonRpcErrResponse struct {
	Version string      `json:"jsonrpc"`
	Id      interface{} `json:"id"`
	Error   struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// methodFromRequest returns method from JSON RPC request. It returns '-' if method is empty.
func methodFromRequest(msg []byte) string {
	var req JsonRpcRequest
	json.Unmarshal(msg, &req) // we ignoring unmarshal errors

	if req.Method == "" {
		return "-"
	}

	return req.Method
}

// NewJsonRpcErrResponse returns new JsonRPC err object with correct ID from postData.
// If httpCode is set then it will be multiply by -1.
func NewJsonRpcErrResponse(postData []byte, httpCode int, err error) (rpcErr *JsonRpcErrResponse) {
	// parse json rpc request
	var req JsonRpcRequest
	if mErr := json.Unmarshal(postData, &req); mErr != nil {
		log.Printf("requested message isn't in JsonRpcRequest format: err=%s", mErr)
		return
	}

	rpcErr = &JsonRpcErrResponse{
		Id:      req.Id,
		Version: "2.0",
	}

	// TODO(sergeyfast) err could disclose internal dest rpc urls.
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
		log.Println(err)
	}

	return resp
}

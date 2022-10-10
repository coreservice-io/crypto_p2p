package peer

import (
	"errors"
	"math/rand"
	"sync"

	"github.com/coreservice-io/crypto_p2p/msg"
)

type Request struct {
	Request_id uint32
	Success    chan struct{}
	msg_resp   *msg.MsgResp
}

var request_mgr = &RequestMgr{}

func GetRequestMgr() *RequestMgr {
	return request_mgr
}

type RequestMgr struct {
	requests sync.Map // map[request_id]=> *Request
}

func (req_mgr *RequestMgr) NewRequest() (*Request, error) {

	req := &Request{
		Request_id: rand.Uint32(),
	}

	_, loeaded := request_mgr.requests.LoadOrStore(req.Request_id, req)
	if loeaded {
		return nil, errors.New("random error")
	}

	return req, nil
}

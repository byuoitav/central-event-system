package httpbuffer

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/byuoitav/common/log"
)

//HTTPBuffer is responsible for sending events to locations that only accept an http request, as opposed to the messenger protocol
type HTTPBuffer struct {
	incomingBuffer chan eventWrapper
	timeout        time.Duration
}

type eventWrapper struct {
	event  []byte
	addr   string
	method string
}

//Status .
type Status struct {
	BufferCap  int `json:"buffer-cap"`
	BufferUtil int `json:"buffer-util"`
}

//SendEvent .
func (h *HTTPBuffer) SendEvent(event []byte, method, address string) {
	h.incomingBuffer <- eventWrapper{event, address, method}
}

//GetStatus .
func (h *HTTPBuffer) GetStatus() Status {
	return Status{
		BufferCap:  cap(h.incomingBuffer),
		BufferUtil: len(h.incomingBuffer),
	}
}

//New .
func New(timeout time.Duration, bufferSize int) *HTTPBuffer {
	toReturn := &HTTPBuffer{
		timeout:        timeout,
		incomingBuffer: make(chan eventWrapper, bufferSize),
	}

	go toReturn.run()
	return toReturn
}

func (h *HTTPBuffer) run() {
	c := http.Client{
		Timeout: h.timeout,
	}
	for {
		wrap := <-h.incomingBuffer

		log.L.Debugf("Sending event to %v with method %v", wrap.addr, wrap.method)

		req, err := http.NewRequest(wrap.method, wrap.addr, bytes.NewReader(wrap.event))
		if err != nil {
			log.L.Errorf("Couldn't build request for %v : %v", wrap.addr, err.Error())
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.Do(req)
		if err != nil {
			log.L.Errorf("Couldn't send event to %v : %v", wrap.addr, err.Error())
			continue
		}

		respb, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.L.Errorf("Couldn't read response body from %v: %v", wrap.addr, err.Error())
			continue
		}

		if resp.StatusCode/100 != 2 {
			log.L.Warnf("Non-w00 received from %v. Code: %v, body: %s", wrap.addr, resp.StatusCode, respb)
		} else {
			log.L.Debugf("response: %s", respb)
		}
	}
}

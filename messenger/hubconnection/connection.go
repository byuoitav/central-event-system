package hubconnection

//TODO: This is basically a copy of the event node read/write pump, just with a different data type stuff.

import (
	"github.com/byuoitav/central-event-system/hub/base"
	"github.com/gorilla/websocket"
)

//HubConnection is the connection from this receiver to a hub
type HubConnection struct {
	ID           string
	writeChannel chan base.EventWrapper
	readChannel  chan base.EventWrapper
	conn         *websocket.Conn
}

//SendEvent will queue an event to be sent to the central hub
func (h *HubConnection) SendEvent(b base.EventWrapper) {
	h.writeChannel <- b
}

//ReadEvent requests the next available event from the queue
func (h *HubConnection) ReadEvent() base.EventWrapper {
	return <-h.readChannel
}

func (h *HubConnection) startReadPump() {

}

func (h *HubConnection) startWritePump() {

}

package incomingconnection

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/byuoitav/central-event-system/hub/base"
	"github.com/byuoitav/central-event-system/hub/nexus"
	"github.com/byuoitav/common/log"
	"github.com/fatih/color"
	"github.com/gorilla/websocket"
)

//Pings are initalized by the hub.
const (
	// Time allowed to write a message to the peer.
	WriteWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	PongWait = 60 * time.Second

	// Time allowed to read the next pong message from the router.
	PingWait = 90 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	PingPeriod = (PongWait * 5) / 10
)

//Connections is the map of all active connections - used mostly for monitoring
var (
	Connections map[string]*IncomingConnection
	upgrader    = websocket.Upgrader{
		ReadBufferSize:  2048,
		WriteBufferSize: 2048,
	}
)

func init() {
	Connections = make(map[string]*IncomingConnection)
}

//IncomingConnection represents a connection from the Hub to either a Hub, Spoke, Ingester, or Dispatcher
type IncomingConnection struct {
	Type  string
	ID    string
	Rooms []string

	WriteChannel chan base.EventWrapper
	ReadChannel  chan base.EventWrapper

	conn  *websocket.Conn
	nexus *nexus.Nexus
}

//CreateConnection promotes a regular http connection to a websocket, starts the read/write pumps, and registers it with the nexus
func CreateConnection(resp http.ResponseWriter, req *http.Request, connType string, nexus *nexus.Nexus) error {
	conn, err := upgrader.Upgrade(resp, req, nil)
	if err != nil {
		log.L.Errorf("Couldn't upgrade	Connection to a websocket: %v", err.Error())
		return err
	}
	hubConn := &IncomingConnection{
		Type:         connType,
		ID:           req.RemoteAddr + connType,
		WriteChannel: make(chan base.EventWrapper, 1000),
		ReadChannel:  make(chan base.EventWrapper, 5000),

		conn:  conn,
		nexus: nexus,
	}

	//we need to register ourselves
	nexus.RegisterConnection([]string{}, hubConn.WriteChannel, hubConn.ID, connType)

	go hubConn.startReadPump()
	hubConn.startWritePump()
	return nil

}

func (h *IncomingConnection) startReadPump() {

	defer func() {
		log.L.Infof(color.HiBlueString("[%v] read pump closing", h.ID))
		h.nexus.DeregisterConnection(h.Rooms, h.Type, h.ID)
		h.conn.Close()
	}()

	h.conn.SetReadDeadline(time.Now().Add(PongWait))
	h.conn.SetPongHandler(func(string) error {
		log.L.Infof("[%v] pong", h.ID)
		h.conn.SetReadDeadline(time.Now().Add(PongWait))
		return nil
	})

	//Spokes are the only ones that will send subscription messages, dispatchers and hubs won't
	for {
		messageType, b, err := h.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway) {
				log.L.Errorf("Websocket closing unexpectedly: %v", err)
				return
			}
			log.L.Errorf("Error with Read Pump for %v: %v", h.ID, err)
			return
		}
		if h.Type == base.Messenger && messageType == websocket.TextMessage {
			//we assume that is'a subscription change
			var change base.RegistrationChange
			err := json.Unmarshal(b, &change)
			if err != nil {
				log.L.Errorf("Invalid registration change received %s", b)
				continue
			}

			//else we submit the subscription chagne
			change.Type = h.Type
			change.ID = h.ID
			change.Channel = h.WriteChannel

			h.nexus.SubmitRegistrationChange(change)
		} else {
			h.ingestMessage(b)
		}
	}
}

func (h *IncomingConnection) startWritePump() {
	log.L.Infof("Starting write pump with a ping timer of %v", PingPeriod)
	ticker := time.NewTicker(PingPeriod)

	defer func() {
		log.L.Infof("Write pump for %v closing...", h.ID)
		ticker.Stop()
		h.conn.Close()
	}()

	for {
		select {
		case message, ok := <-h.WriteChannel:
			h.conn.SetWriteDeadline(time.Now().Add(WriteWait))
			if !ok {
				// The hub closed the channel.
				h.conn.WriteControl(websocket.CloseMessage, []byte{}, time.Now().Add(WriteWait))
				return
			}

			//write
			err := h.conn.WriteMessage(websocket.BinaryMessage, base.PrepareMessage(message))
			if err != nil {
				log.L.Errorf("%v Error %v", h.ID, err.Error())
				return
			}

		case <-ticker.C:
			h.conn.SetWriteDeadline(time.Now().Add(WriteWait))
			log.L.Infof("[%v] Sending ping.", h.ID)
			if err := h.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(WriteWait)); err != nil {
				return
			}
		}

	}
}

/*
Ingest message assumes an event in the format of:
RoomID\n
JSONEvent
*/
func (h *IncomingConnection) ingestMessage(b []byte) {
	m, err := base.ParseMessage(b)
	if err != nil {
		log.L.Warnf("Received badly formed event %s: %v", b, err.Error())
		return
	}
	h.nexus.Submit(
		m,
		h.Type,
		h.ID,
	)
}

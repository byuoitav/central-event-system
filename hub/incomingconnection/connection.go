package incomingconnection

import (
	"bytes"
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
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Time allowed to read the next pong message from the router.
	pingWait = 90 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 5) / 10
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
		ID:           req.RemoteAddr + req.URL.Path,
		WriteChannel: make(chan base.EventWrapper, 1000),
		ReadChannel:  make(chan base.EventWrapper, 5000),

		conn: conn,
	}

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

	h.conn.SetReadDeadline(time.Now().Add(pongWait))
	h.conn.SetPongHandler(func(string) error {
		log.L.Infof("[%v] pong", h.ID)
		h.conn.SetReadDeadline(time.Now().Add(pongWait))
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
			//it's a subscription change
		} else {
			h.ingestMessage(b)
		}
	}
}

func (h *IncomingConnection) startWritePump() {
	log.L.Infof("Starting write pump with a ping timer of %v", pingPeriod)
	ticker := time.NewTicker(pingPeriod)

	defer func() {
		log.L.Infof("Write pump for %v closing...", h.ID)
		ticker.Stop()
		h.conn.Close()
	}()

	for {
		select {
		case message, ok := <-h.WriteChannel:
			h.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				h.conn.WriteControl(websocket.CloseMessage, []byte{}, time.Now().Add(writeWait))
				return
			}

			//write
			err := h.conn.WriteMessage(websocket.BinaryMessage, append([]byte(message.Room+"\n"), message.Event...))
			if err != nil {
				log.L.Errorf("%v Error %v", h.ID, err.Error())
				return
			}

		case <-ticker.C:
			h.conn.SetWriteDeadline(time.Now().Add(writeWait))
			log.L.Infof("[%v] Sending ping.", h.ID)
			if err := h.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait)); err != nil {
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

	//parse out room name
	index := bytes.IndexByte(b, '\n')
	if index == -1 {
		log.L.Errorf("Invalid message format: %v", b)
		return
	}

	h.nexus.Submit(base.EventWrapper{
		Room:  string(b[:index]),
		Event: b[index:],
	},
		h.Type,
		h.ID,
	)
}

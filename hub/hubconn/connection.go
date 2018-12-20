package hubconn

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/byuoitav/central-event-system/hub/base"
	"github.com/byuoitav/central-event-system/hub/nexus"
	"github.com/byuoitav/common/log"
	"github.com/byuoitav/common/nerr"
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
	Connections map[string]*connection
	upgrader    = websocket.Upgrader{
		ReadBufferSize:  2048,
		WriteBufferSize: 2048,
	}
)

func init() {
	Connections = map[string]*connection{}
}

//connection represents a connection from the Hub to either a Hub, Spoke, Ingester, or Dispatcher
type connection struct {
	Type  string
	ID    string
	Rooms []string

	WriteChannel chan base.EventWrapper
	ReadChannel  chan base.EventWrapper
	exitChan     chan bool
	retry        bool // will try to reconnect if set to true
	addr         string
	path         string
	connType     string

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
	hubConn := &connection{
		Type:         connType,
		ID:           req.RemoteAddr + connType,
		WriteChannel: make(chan base.EventWrapper, 1000),
		ReadChannel:  make(chan base.EventWrapper, 5000),
		exitChan:     make(chan bool, 2),

		conn:  conn,
		nexus: nexus,
	}

	//we need to register ourselves
	nexus.RegisterConnection([]string{}, hubConn.WriteChannel, hubConn.ID, connType)

	go hubConn.startReadPump()
	hubConn.startWritePump()
	return nil

}

//OpenConnectionWithRetry reaches out to another central event system and establishes a websocket with it, and then registers it with the nexus
//Do not include protocol with addr,  path will have all leading and trailing `/` characters removed
func OpenConnectionWithRetry(addr string, path string, connType string, nexus *nexus.Nexus) error {
	log.L.Infof("attempting to open connection with %v %v.", connType, addr)
	MaxBackoff := 120 * time.Second //Wait at most 60 seconds between retrys
	curBackoff := 2 * time.Second   //Start by waiting for 2 seconds
	t := 0
	l := 5

	err := OpenConnection(addr, path, connType, nexus, true)

	for err != nil {
		log.L.Infof("connection to %v %v failed. Will retry in %s. ", connType, addr, curBackoff.String())
		time.Sleep(curBackoff)
		err = OpenConnection(addr, path, connType, nexus, true)
		if err != nil {
			if t >= l {
				t = 0
				if curBackoff < MaxBackoff {
					curBackoff = (curBackoff / 2) + curBackoff
				}
				if curBackoff > MaxBackoff {
					curBackoff = MaxBackoff
				}
			} else {
				t++
			}
		}
	}

	return nil
}

//OpenConnection reaches out to another central event system and establishes a websocket with it, and then registers it with the nexus
//Do not include protocol with addr,  path will have all leading and trailing `/` characters removed
func OpenConnection(addr string, path string, connType string, nexus *nexus.Nexus, retry bool) error {
	// open connection to the router
	dialer := &websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	path = strings.Trim(path, "/")

	conn, _, err := dialer.Dial(fmt.Sprintf("ws://%s/%s", addr, path), nil)
	if err != nil {
		return nerr.Create(fmt.Sprintf("failed opening websocket with %v: %s", addr, err), "connection-error")
	}

	hubConn := &connection{
		Type:         connType,
		ID:           conn.RemoteAddr().String() + connType,
		WriteChannel: make(chan base.EventWrapper, 1000),
		ReadChannel:  make(chan base.EventWrapper, 5000),
		exitChan:     make(chan bool, 2),
		retry:        retry,
		addr:         addr,
		path:         path,
		connType:     connType,

		conn:  conn,
		nexus: nexus,
	}

	//we need to register ourselves
	nexus.RegisterConnection([]string{}, hubConn.WriteChannel, hubConn.ID, connType)

	go hubConn.startReadPump()
	go hubConn.startWritePump()
	return nil

}

func (h *connection) startReadPump() {

	defer func() {
		log.L.Infof(color.HiBlueString("[%v] read pump closing", h.ID))
		h.nexus.DeregisterConnection(h.Rooms, h.Type, h.ID)
		h.exitChan <- true
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

func (h *connection) startWritePump() {
	log.L.Infof("Starting write pump with a ping timer of %v", PingPeriod)
	ticker := time.NewTicker(PingPeriod)

	defer func() {
		log.L.Infof("Write pump for %v closing...", h.ID)
		ticker.Stop()
		h.conn.Close()
		if h.retry {
			log.L.Infof("Connection %v is set for retry, will attempt to re-establish connection", h.ID)
			go OpenConnectionWithRetry(h.addr, h.path, h.connType, h.nexus)
		}
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
		case <-h.exitChan:
			h.conn.WriteControl(websocket.CloseMessage, []byte{}, time.Now().Add(WriteWait))
			return

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
func (h *connection) ingestMessage(b []byte) {
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

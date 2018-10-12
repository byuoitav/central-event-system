package hubconnection

import (
	"net/http"
	"time"

	"github.com/byuoitav/central-event-system/hub/axle"
	"github.com/byuoitav/central-event-system/hub/base"
	"github.com/byuoitav/common/log"
	"github.com/fatih/color"
	"github.com/gorilla/websocket"
)

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
	Connections map[string]*HubConnection
	upgrader    = websocket.Upgrader{
		ReadBufferSize:  2048,
		WriteBufferSize: 2048,
	}
)

func init() {
	Connection = make(map[string]*HubConnection)
}

//HubConnection represents a connection from the Hub to either a Hub, Spoke, Ingester, or Dispatcher
type HubConnection struct {
	Type  string
	ID    string
	Rooms []string

	WriteChannel chan base.EventWrapper
	ReadChannel  chan base.EventWrapper

	conn *websocket.Conn
	axle *axle.Axle
}

//CreateHubConnection promotes a regular http connection to a websocket, starts the read/write pumps, and registers it with the axle
func CreateHubConnection(resp http.ResponseWriter, req *http.Request, connType string, axle *axle.Axle) error {
	conn, err := upgrader.Upgrade(resp, req, nil)
	if err != nil {
		log.L.Errorf("Couldn't upgrade	Connection to a websocket: %v", err.Error())
		return err
	}

	hubConn := &HubConnection{
		Type:         connType,
		ID:           req.RemoteAddr + req.URL.String(),
		WriteChannel: make(chan base.EventWrapper, 1000),
		ReadChannel:  make(chan base.EventWrapper, 5000),

		conn: conn,
	}

}

func (h *HubConnection) startReadPump() {

	defer func() {
		log.L.Infof(color.HiBlueString("[%v] read pump closing", h.ID))
		h.axle.UnregisterConnection(h.Rooms, h.Type, h.ID)
		h.conn.Close()
	}()

	h.conn.SetReadDeadline(time.Now().Add(pongWait))
	h.conn.SetPongHandler(func(string) error {
		log.L.Infof(color.HiCyanString("[%v] pong", h.ID))
		h.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	//Spokes are the only ones that will send subscription messages, dispatchers and hubs won't
	if h.Type == base.Spoke {
		for {
			messageType, bytes, err = h.conn.ReadMessage(i)
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway) {
					log.Printf("error: %v", err)
				}
				break
			}
			if messageType == websocket.TextMessage {
				//it's a subscription change
				//we need to do the parsing to grab the room name out, then send it to the axle
				s.router.inChan <- message
			}
		}
	} else {

	}
}

func (h *HubConnection) startWritePump() {

}

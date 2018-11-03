package messenger

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/byuoitav/central-event-system/hub/base"
	"github.com/byuoitav/central-event-system/hub/hubconn"
	"github.com/byuoitav/common/log"
	"github.com/byuoitav/common/nerr"
	"github.com/byuoitav/common/v2/events"
	"github.com/fatih/color"
	"github.com/gorilla/websocket"
)

const (
	// Interval to wait between retry attempts
	retryInterval = 3 * time.Second
)

//Messenger is the connection from this receiver to a hub
type Messenger struct {
	HubAddr        string
	ConnectionType string

	writeChannel        chan base.EventWrapper
	subscriptionChannel chan base.SubscriptionChange
	readChannel         chan base.EventWrapper

	conn *websocket.Conn

	readDone     chan bool
	writeDone    chan bool
	lastPingTime time.Time
	state        string
}

//SendEvent will queue an event to be sent to the central hub
func (h *Messenger) SendEvent(e events.Event) {
	h.Send(base.WrapEvent(e))
}

//Send .
func (h *Messenger) Send(b base.EventWrapper) {
	h.writeChannel <- b
}

//ReceiveEvent requests the next available event from the queue
func (h *Messenger) ReceiveEvent() events.Event {
	var e events.Event
	err := json.Unmarshal(h.Receive().Event, &e)
	if err != nil {
		log.L.Errorf("Invalid event received: %v", err.Error())
	}
	return e
}

//Receive .
func (h *Messenger) Receive() base.EventWrapper {
	return <-h.readChannel
}

//SubscribeToRooms .
func (h *Messenger) SubscribeToRooms(r ...string) {
	if len(r) == 0 {
		return
	}

	h.subscriptionChannel <- base.SubscriptionChange{
		Rooms:  r,
		Create: true,
	}

}

//UnsubscribeFromRooms .
func (h *Messenger) UnsubscribeFromRooms(r ...string) {
	if len(r) < 1 {
		return
	}

	h.subscriptionChannel <- base.SubscriptionChange{
		Rooms:  r,
		Create: false,
	}
}

//BuildMessenger starts a connection to the hub provided, and then returns the connection (messenger)
func BuildMessenger(HubAddress, connectionType string, bufferSize int) (*Messenger, *nerr.E) {
	log.L.Infof("starting messenger with %v, connection type %v, buffer size %v", HubAddress, connectionType, bufferSize)
	h := &Messenger{
		HubAddr:             HubAddress,
		ConnectionType:      connectionType,
		writeChannel:        make(chan base.EventWrapper, bufferSize),
		subscriptionChannel: make(chan base.SubscriptionChange, 100),
		readChannel:         make(chan base.EventWrapper, bufferSize),
		readDone:            make(chan bool, 1),
		writeDone:           make(chan bool, 1),
	}

	// open connection with router
	err := h.openConnection()
	if err != nil {
		log.L.Warnf("Opening connection to hub failed: %v, retrying...", err.Error())

		h.readDone <- true
		h.writeDone <- true
		go h.retryConnection()

		return h, nerr.Create(fmt.Sprintf("failed to open connection to hub %v. retrying connection...", h.HubAddr), "retrying")
	}

	// update state to good
	h.state = "good"
	log.L.Infof(color.HiGreenString("Successfully connected to hub %s. Starting pumps...", h.HubAddr))

	// start read/write pumps
	go h.startReadPump()
	go h.startWritePump()

	return h, nil
}

func (h *Messenger) openConnection() error {
	// open connection to the router
	dialer := &websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.Dial(fmt.Sprintf("ws://%s/connect/%s", h.HubAddr, h.ConnectionType), nil)
	if err != nil {
		return nerr.Create(fmt.Sprintf("failed opening websocket with %v: %s", h.HubAddr, err), "connection-error")
	}

	h.conn = conn
	return nil
}

func (h *Messenger) retryConnection() {
	// mark the connection as 'down'
	h.state = h.state + " retrying"

	log.L.Infof("[retry] Retrying connection, waiting for read and write pump to close before starting.")
	//wait for read to say i'm done.
	<-h.readDone
	log.L.Infof("[retry] Read pump closed")

	//wait for write to be done.
	<-h.writeDone
	log.L.Infof("[retry] Write pump closed")
	log.L.Infof("[retry] Retrying connection")

	//we retry
	err := h.openConnection()

	for err != nil {
		log.L.Infof("[retry] Retry failed, trying to connect to %s again in %v seconds.", h.HubAddr, retryInterval)
		time.Sleep(retryInterval)
		err = h.openConnection()
	}

	//start the pumps again
	log.L.Infof(color.HiGreenString("[Retry] Retry success. Starting pumps"))

	h.state = "good"
	go h.startReadPump()
	go h.startWritePump()

}

func (h *Messenger) startReadPump() {
	defer func() {
		h.conn.Close()
		log.L.Warnf("Connection to hub %v is dying.", h.HubAddr)
		h.state = "down"

		h.readDone <- true
	}()

	h.conn.SetPingHandler(
		func(string) error {
			log.L.Infof("[%v] Ping!", h.HubAddr)
			h.conn.SetReadDeadline(time.Now().Add(hubconn.PingWait))
			h.conn.WriteControl(websocket.PongMessage, []byte{}, time.Now().Add(hubconn.WriteWait))

			//debugging purposes
			h.lastPingTime = time.Now()

			return nil
		})

	h.conn.SetReadDeadline(time.Now().Add(hubconn.PingWait))

	for {
		t, b, err := h.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway) {
				log.L.Errorf("Websocket closing: %v", err)
			}
			log.L.Errorf("Error: %v", err)
			return
		}

		if t != websocket.BinaryMessage {
			log.L.Warnf("Unknown message type %v", t)
			continue
		}

		//parse out room name
		m, er := base.ParseMessage(b)
		if er != nil {
			log.L.Warnf("Poorly formed message %s: %v", b, er.Error())
			continue
		}
		h.readChannel <- m
	}

}

func (h *Messenger) startWritePump() {
	defer func() {
		h.conn.Close()
		log.L.Warnf("Connection to hub %v is dying. Trying to resurrect.", h.HubAddr)
		h.state = "down"

		h.writeDone <- true

		//try to reconnect
		h.retryConnection()
	}()

	for {
		select {
		case message, ok := <-h.writeChannel:
			if !ok {
				h.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(hubconn.WriteWait))
				return
			}

			err := h.conn.WriteMessage(websocket.BinaryMessage, base.PrepareMessage(message))
			if err != nil {
				log.L.Errorf("Problem writing message to socket: %v", err.Error())
				return
			}

		case _, ok := <-h.readDone:
			if !ok {
				h.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(hubconn.WriteWait))
				return
			}
			// put it back in
			h.readDone <- true
			return

		case s, ok := <-h.subscriptionChannel:
			if !ok {
				h.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(hubconn.WriteWait))
				return
			}
			b, err := json.Marshal(s)
			if err != nil {
				log.L.Errorf("Couldn't marshal subscription change: %v", err.Error())
				continue
			}
			err = h.conn.WriteMessage(websocket.TextMessage, b)
			if err != nil {
				log.L.Errorf("Problem writing message to socket: %v", err.Error())
				return
			}

		}
	}

}

// GetState returns the state of the messenger connection to the hub.
func (h *Messenger) GetState() interface{} {
	values := make(map[string]interface{})

	values["hub"] = h.HubAddr

	if h.conn != nil {
		values["connection"] = fmt.Sprintf("%v => %v", h.conn.LocalAddr().String(), h.conn.RemoteAddr().String())
	} else {
		values["connection"] = fmt.Sprintf("%v => %v", "local", h.HubAddr)
	}

	values["state"] = h.state
	values["last-ping-time"] = h.lastPingTime.Format(time.RFC3339)
	return values
}

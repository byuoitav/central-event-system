package messenger

import (
	"fmt"
	"time"

	"github.com/byuoitav/central-event-system/hub/base"
	"github.com/byuoitav/central-event-system/hub/incomingconnection"
	"github.com/byuoitav/common/log"
	"github.com/byuoitav/common/nerr"
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

	writeChannel chan base.EventWrapper
	readChannel  chan base.EventWrapper

	conn *websocket.Conn

	readDone     chan bool
	writeDone    chan bool
	lastPingTime time.Time
	state        string
}

//SendEvent will queue an event to be sent to the central hub
func (h *Messenger) SendEvent(b base.EventWrapper) {
	h.writeChannel <- b
}

//ReceiveEvent requests the next available event from the queue
func (h *Messenger) ReceiveEvent() base.EventWrapper {
	return <-h.readChannel
}

//BuildMessenger starts a connection to the hub provided, and then returns the connection (messenger)
func BuildMessenger(HubAddress, connectionType string, bufferSize int) (*Messenger, *nerr.E) {
	h := &Messenger{
		HubAddr:        HubAddress,
		ConnectionType: connectionType,
		writeChannel:   make(chan base.EventWrapper, bufferSize),
		readChannel:    make(chan base.EventWrapper, bufferSize),
	}

	// open connection with router
	err := h.openConnection()
	if err != nil {
		log.L.Warnf("Opening connection to hub failed, retrying...")

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
			h.conn.SetReadDeadline(time.Now().Add(incomingconnection.PingWait))
			h.conn.WriteControl(websocket.PongMessage, []byte{}, time.Now().Add(incomingconnection.WriteWait))

			//debugging purposes
			h.lastPingTime = time.Now()

			return nil
		})

	h.conn.SetReadDeadline(time.Now().Add(incomingconnection.PingWait))

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
				h.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(incomingconnection.WriteWait))
				return
			}

			err := h.conn.WriteMessage(websocket.BinaryMessage, base.PrepareMessage(message))
			if err != nil {
				log.L.Errorf("Problem writing message to socket: %v", err.Error())
				return
			}

		case <-h.readDone:
			// put it back in
			h.readDone <- true
			return
		}
	}

}

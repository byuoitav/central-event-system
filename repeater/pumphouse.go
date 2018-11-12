package main

import (
	"fmt"
	"net"
	"time"

	"github.com/byuoitav/central-event-system/hub/base"
	"github.com/byuoitav/common/db"
	"github.com/byuoitav/common/log"
	"github.com/byuoitav/common/nerr"
	"github.com/gorilla/websocket"
)

//Pings are initalized by the hub.
const (
	TTL = 5 * time.Second
	//readBufferSize
	readBufferSize  = 1024
	writeBufferSize = 1024

	//port for the translators on the devices
	translatorport = "7101"

	//WriteWait Time allowed to write a message to the peer.
	WriteWait = 10 * time.Second

	//PongWait Time allowed to read the next pong message from the peer.
	PongWait = 60 * time.Second

	//PingWait Time allowed to read the next pong message from the router.
	PingWait = 90 * time.Second

	//PingPeriod Send pings to peer with this period. Must be less than pongWait.
	PingPeriod = (PongWait * 5) / 10
)

//PumpingStation .
type PumpingStation struct {
	conn *websocket.Conn

	ID   string
	Room string

	remoteaddr string
	starttime  time.Time

	//internal channels
	readChannel  chan base.EventWrapper
	writeChannel chan base.EventWrapper

	readExit  chan bool
	writeExit chan bool
	pingExit  chan bool

	writeConfirm chan bool
	timeoutChan  chan bool
	errorChan    chan error

	writeTimeout time.Time
	readTimeout  time.Time

	//external channels
	ReceiveChannel chan base.EventWrapper
	SendChannel    chan base.EventWrapper

	dbDevConn bool
	tick      bool //if we initialized the connection or not. Only those who initialize start a ticker

	r *Repeater
}

//PumpingStationStatus .
type PumpingStationStatus struct {
	RemoteAddress string `json:"remote-addr"`

	Uptime time.Duration

	ReadBufferPublicCap   int `json:"read-buffer-public-cap"`
	ReadBufferPublicUtil  int `json:"read-buffer-public-util"`
	ReadBufferPrivateCap  int `json:"read-buffer-private-cap"`
	ReadBufferPrivateUtil int `json:"read-buffer-private-util"`

	WriteBufferPublicCap   int `json:"write-buffer-public-cap"`
	WriteBufferPublicUtil  int `json:"write-buffer-public-util"`
	WriteBufferPrivateCap  int `json:"write-buffer-private-cap"`
	WriteBufferPrivateUtil int `json:"write-buffer-private-util"`

	ReadTimeout  time.Time `json:"read-timeout"`
	WriteTimeout time.Time `json:"write-timeout"`
}

//GetStatus .
func (c *PumpingStation) GetStatus() PumpingStationStatus {
	return PumpingStationStatus{
		RemoteAddress:          c.remoteaddr,
		Uptime:                 time.Since(c.starttime),
		ReadBufferPublicCap:    cap(c.ReceiveChannel),
		ReadBufferPublicUtil:   len(c.ReceiveChannel),
		ReadBufferPrivateCap:   cap(c.readChannel),
		ReadBufferPrivateUtil:  len(c.readChannel),
		WriteBufferPublicCap:   cap(c.SendChannel),
		WriteBufferPublicUtil:  len(c.SendChannel),
		WriteBufferPrivateCap:  cap(c.writeChannel),
		WriteBufferPrivateUtil: len(c.writeChannel),
		ReadTimeout:            c.readTimeout,
		WriteTimeout:           c.writeTimeout,
	}
}

//StartConnection takes a proc number, and will build the buffers, return it while asyncronously starting the connection
func StartConnection(proc, room string, r *Repeater, dbDevConn bool) (*PumpingStation, *nerr.E) {

	toreturn := &PumpingStation{
		readChannel:    make(chan base.EventWrapper, readBufferSize),
		writeChannel:   make(chan base.EventWrapper, writeBufferSize),
		ReceiveChannel: r.HubSendBuffer,
		SendChannel:    make(chan base.EventWrapper, writeBufferSize),
		readExit:       make(chan bool, 1),
		pingExit:       make(chan bool, 1),
		timeoutChan:    make(chan bool, 1),
		writeExit:      make(chan bool, 1),
		writeConfirm:   make(chan bool, 1),
		errorChan:      make(chan error, 6),
		ID:             proc,
		dbDevConn:      dbDevConn, //is this a device we need to get from the database?
		Room:           room,
		r:              r,
		tick:           true,
		starttime:      time.Now(),
	}

	go toreturn.start()

	return toreturn, nil
}

func buildFromConnection(proc, room string, r *Repeater, conn *websocket.Conn) (*PumpingStation, *nerr.E) {

	toreturn := &PumpingStation{
		readChannel:    make(chan base.EventWrapper, readBufferSize),
		writeChannel:   make(chan base.EventWrapper, writeBufferSize),
		ReceiveChannel: r.HubSendBuffer,
		SendChannel:    make(chan base.EventWrapper, writeBufferSize),
		readExit:       make(chan bool, 1),
		pingExit:       make(chan bool, 1),
		timeoutChan:    make(chan bool, 1),
		writeExit:      make(chan bool, 1),
		writeConfirm:   make(chan bool, 1),
		errorChan:      make(chan error, 2),
		ID:             proc,
		Room:           room,
		r:              r,
		conn:           conn,
		remoteaddr:     conn.RemoteAddr().String(),
		tick:           false,
		starttime:      time.Now(),
	}

	go toreturn.startReadPump()
	go toreturn.startWritePump()

	//we assume that the caller will start the pumper
	return toreturn, nil
}

func (c *PumpingStation) start() {
	log.L.Debugf("Starting pumping station...")
	addr := ""
	if c.dbDevConn {
		//we need to get the address of the processor I want to talk to a
		dev, err := db.GetDB().GetDevice(c.ID)
		if err != nil {
			log.L.Errorf("Couldn't retrieve device %v from database: %v", c.ID, err.Error())
			c.r.UnregisterConnection(c.ID)
			return
		}
		addr = dev.Address
	} else {
		addr = c.ID
	}

	err := c.openConn(addr)
	if err != nil {
		log.L.Errorf("couldn't initializle for %v: %v", c.ID, err.Error())
		c.r.UnregisterConnection(c.ID)
		return
	}

	log.L.Infof("[%v] Connection to %v established, starting pumps...", c.ID, addr)

	go c.startReadPump()
	go c.startWritePump()
	go c.startPing()

	c.startPumper()
}

func (c *PumpingStation) openConn(addr string) *nerr.E {
	log.L.Debugf("Starting connection with %v", addr)

	//check if addr has a port
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		log.L.Debugf("Couldn't split port %v", err)
		addr = addr + ":" + translatorport
	} else {
		if port == "" {
			addr = addr + ":" + translatorport
		}
	}

	c.remoteaddr = addr

	dialer := &websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	fulladdr := fmt.Sprintf("ws://%s/connect/%s/%s", addr, c.Room, c.r.RepeaterID)
	log.L.Debugf("Connecting to: %v", fulladdr)

	conn, _, err := dialer.Dial(fulladdr, nil)
	if err != nil {
		return nerr.Create(fmt.Sprintf("failed opening websocket with %v: %s", addr, err), "connection-error")
	}
	log.L.Debugf("Connection started with %v", addr)

	c.conn = conn
	return nil
}

//We don't try to re-establish this one, nor do we worry about ping/pong joy - we're alive until one of us closes it - hopefully 5 seconds of inactivity
func (c *PumpingStation) startReadPump() {

	defer func() {
		if r := recover(); r != nil {
			//clean up the connection
			log.L.Debugf("[%v] recovering from panic: %v", c.ID, r)
			c.errorChan <- fmt.Errorf("%v", r)
			return
		}
	}()

	for {
		_, b, err := c.conn.ReadMessage()
		if err != nil {
			if er, ok := err.(*websocket.CloseError); ok {
				log.L.Debugf("[%v] Websocket closing: %v", c.ID, er)
				c.errorChan <- err
				return
			}

			log.L.Debugf("[%v] Returning: %v", c.ID, err)
			c.errorChan <- err
			return
		}

		msg, er := base.ParseMessage(b)
		if er != nil {
			log.L.Errorf("Couldn't parse message %s.", er)
		}
		c.readChannel <- msg
	}
}

func (c *PumpingStation) startWritePump() {

	defer func() {
		c.writeConfirm <- true
	}()
	var msg base.EventWrapper

	for {
		select {
		case msg = <-c.writeChannel:
			c.conn.SetWriteDeadline(time.Now().Add(WriteWait))
			//in the case of the write channel we just write it down the socket
			err := c.conn.WriteMessage(websocket.BinaryMessage, base.PrepareMessage(msg))
			if err != nil {
				log.L.Debugf("[%v} Problem writing message: %v", c.ID, err.Error())
				c.errorChan <- err
				return
			}

		case <-c.writeExit:
			return

		case <-c.timeoutChan:
			log.L.Infof("[%v] Timeout. Closing.", c.ID)
			err := c.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseGoingAway, ""), time.Now().Add(500*time.Millisecond))
			if err != nil {
				log.L.Debugf("[%v} Problem writing message: %v", c.ID, err.Error())
				c.errorChan <- err
			}
			return
		}
	}
}

func (c *PumpingStation) startPumper() {

	c.conn.SetPongHandler(
		func(string) error {
			c.conn.SetReadDeadline(time.Now().Add(PongWait))
			return nil
		})

	log.L.Debug("Starting pumper...")
	defer func() {
		c.r.UnregisterConnection(c.ID)

		c.writeExit <- true
		c.readExit <- true
		c.pingExit <- true

		<-c.writeConfirm

		err := c.conn.Close()
		if err != nil {
			log.L.Errorf("Couldn't close the connection.")
		}

		time.Sleep(TTL)
	}()

	c.readTimeout = time.Now().Add(TTL)
	c.writeTimeout = time.Now().Add(TTL)

	//start our ticker
	t := time.NewTicker(TTL)
	if !c.tick {
		//we cancel the ticker
		t.Stop()
	} else {
		log.L.Debugf("Ticker started")
	}
	for {
		select {
		case <-t.C:
			log.L.Debugf("tick. Checking for timeout.")
			//check to see if read and write are after now
			if time.Now().After(c.readTimeout) && time.Now().After(c.writeTimeout) {
				log.L.Debugf("Timeout...")

				//tell the write channel to timeout
				c.timeoutChan <- true

				//time to leave
				return
			}
			log.L.Debugf("No need to close... Continuing")

		case err := <-c.errorChan:
			//there was an error
			log.L.Debugf("[%v] error: %v. Closing..", c.ID, err.Error())
			return

		case e := <-c.SendChannel:
			log.L.Debugf("[%v] Sending message: %s:%s.", c.ID, e.Room, e.Event)

			c.writeTimeout = time.Now().Add(TTL)
			c.writeChannel <- e

		case e := <-c.readChannel:
			log.L.Debugf("[%v] Received message.", c.ID)
			c.readTimeout = time.Now().Add(TTL)
			c.ReceiveChannel <- e
		}
	}

}

//StartPing .
func (c *PumpingStation) startPing() {
	ticker := time.NewTicker(PingPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			err := c.conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(WriteWait))
			if err != nil {
				log.L.Errorf("ping error: %v", err)
				c.errorChan <- err
				return
			}

		case <-c.pingExit:
			return

		}
	}
}

//SendEvent .
func (c *PumpingStation) SendEvent(e base.EventWrapper) {
	c.SendChannel <- e
}

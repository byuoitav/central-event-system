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

const (
	//TTL .
	TTL = 5 * time.Second

	//readBufferSize
	readBufferSize  = 1024
	writeBufferSize = 1024

	//port for the translators on the devices
	translatorport = "7110"

//	translatorport = "7101"
)

//PumpingStation .
type PumpingStation struct {
	conn *websocket.Conn

	ID   string
	Room string

	remoteaddr string

	//internal channels
	readChannel  chan base.EventWrapper
	writeChannel chan base.EventWrapper

	readExit     chan bool
	writeExit    chan bool
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

//StartConnection takes a proc number, and will build the buffers, return it while asyncronously starting the connection
func StartConnection(proc, room string, r *Repeater, dbDevConn bool) (*PumpingStation, *nerr.E) {

	toreturn := &PumpingStation{
		readChannel:    make(chan base.EventWrapper, readBufferSize),
		writeChannel:   make(chan base.EventWrapper, writeBufferSize),
		ReceiveChannel: r.HubSendBuffer,
		SendChannel:    make(chan base.EventWrapper, writeBufferSize),
		readExit:       make(chan bool, 1),
		timeoutChan:    make(chan bool, 1),
		writeExit:      make(chan bool, 1),
		writeConfirm:   make(chan bool, 1),
		errorChan:      make(chan error, 2),
		ID:             proc,
		dbDevConn:      dbDevConn, //is this a device we need to get from the database?
		Room:           room,
		r:              r,
		tick:           true,
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
	}

	go toreturn.startReadPump()
	go toreturn.startWritePump()

	//we assume that the caller will start the pumper
	return toreturn, nil
}

func (c *PumpingStation) start() {
	log.L.Infof("Starting pumping station...")
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

	log.L.Infof("Connection to %v established, starting pumps...", addr)

	go c.startReadPump()
	go c.startWritePump()

	c.startPumper()
}

func (c *PumpingStation) openConn(addr string) *nerr.E {
	log.L.Debugf("Starting connection with %v", addr)

	c.remoteaddr = addr

	dialer := &websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	fulladdr := fmt.Sprintf("ws://%s:%s/connect/%s/%s", addr, translatorport, c.Room, c.r.RepeaterID)
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

	c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		_, b, err := c.conn.ReadMessage()
		if err != nil {
			if er, ok := err.(*websocket.CloseError); ok {
				log.L.Infof("[%v] Websocket closing: %v", c.ID, er)
				c.errorChan <- err
				return
			}
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseMessage, websocket.CloseNormalClosure) {
				log.L.Errorf("[%v] Websocket closing: %v", c.ID, err)
			} else {
				netErr, ok := err.(net.Error)
				if ok && netErr.Timeout() {
					select {
					case <-c.readExit:
						return
					default:
						err := c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
						if err != nil {
							log.L.Warnf("couldn't set read deadline: %v", err.Error())
						}
						continue
					}
				}
			}
			log.L.Debugf("[%v] Returning: %v", c.ID, err)
			c.errorChan <- err
			return
		}

		msg, err := base.ParseMessage(b)
		if err != nil {
			log.L.Errorf("Couldn't parse message %s.", b)
		}
		c.conn.SetReadDeadline(time.Now().Add(TTL))
		c.readChannel <- msg
	}
}

func (c *PumpingStation) startWritePump() {

	defer func() {
		c.writeConfirm <- true
	}()

	c.conn.SetWriteDeadline(time.Now().Add(TTL))
	var msg base.EventWrapper

	for {
		select {
		case msg = <-c.writeChannel:
			//in the case of the write channel we just write it down the socket
			err := c.conn.WriteMessage(websocket.BinaryMessage, base.PrepareMessage(msg))
			if err != nil {
				log.L.Debugf("[%v} Problem writing message: %v", c.ID, err.Error())
				c.errorChan <- err
				return
			}
			c.conn.SetWriteDeadline(time.Now().Add(TTL))

		case <-c.writeExit:
			return
		case <-c.timeoutChan:

			log.L.Infof("Timout. Closing.")

			c.conn.SetWriteDeadline(time.Now().Add(TTL))

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
	log.L.Infof("Starting pumper...")
	defer func() {
		c.r.UnregisterConnection(c.ID)

		c.writeExit <- true
		c.readExit <- true
		<-c.writeConfirm

		time.Sleep(TTL)
		c.conn.Close()
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
			log.L.Infof("No need to close... Continuing")

		case err := <-c.errorChan:
			//there was an error
			log.L.Infof("[%v] error: %v. Closing..", c.ID, err.Error())
			return

		case e := <-c.SendChannel:

			log.L.Debugf("Send Channel: %v", e)

			c.writeTimeout = time.Now().Add(TTL)
			c.writeChannel <- e

		case e := <-c.readChannel:
			log.L.Debugf("Read Channel Channel: %v", e)
			c.readTimeout = time.Now().Add(TTL)
			c.ReceiveChannel <- e
		}
	}

}

//SendEvent .
func (c *PumpingStation) SendEvent(e base.EventWrapper) {
	c.SendChannel <- e
}

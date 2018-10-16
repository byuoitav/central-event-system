package repeater

import (
	"net"
	"time"

	"github.com/byuoitav/common/log"
	"github.com/byuoitav/common/v2/events"
	"github.com/gorilla/websocket"
)

const (
	ttl = 5 //time to live in seconds
)

//ConnectionManager .
type ConnectionManager struct {
	conn websocket.Conn

	ID   string
	Room string

	readChannel  chan event.Event
	writeChannel chan event.Event

	timeoutChan chan bool
	exitChan    chan bool //make at least len 2

	writeTimeout time.Time
	readTimeout  time.Time
}

//We don't try to re-establish this one, nor do we worry about ping/pong joy - we're alive until one of us closes it - hopefully 5 seconds of inactivity
func (c *ConnectionManager) startReadPump() {
	TTL := time.Duration(ttl) * time.Second

	c.conn.SetReadDeadline(time.Now().Add(TTL))
	c.readTimeout = time.Now().Add(TTL)
	for {
		var event events.Event
		t, b, err := c.conn.ReadJSON(&event)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway) {
				log.L.Errorf("[%v] Websocket closing: %v", c.ID, err)
			} else {
				netErr, ok := err.(net.Error)
				if ok && netErr.Timeout() {
					log.L.Debugf("[%v] ReadTimeout", c.ID, err)

					//let the write worker know that we timed out
					timeoutChan <- true

					//wait to see if they want to exit, too
					exit <- exitChan
					if exit {
						log.L.Debugf("[%v] Returning", c.ID, err)
						return
					}

					log.L.Debugf("[%v] Write not done.", c.ID, err)
					c.conn.SetReadDeadline(time.Now().Add(TTL))
					continue
				}
			}
			log.L.Debugf("[%v] Returning", c.ID, err)
			exitChan <- true
			return
		}

		c.readChannel <- m

		c.conn.SetReadDeadline(time.Now().Add(TTL))
		c.readTimeout = time.Now().Add(TTL)
	}

}

func (c *ConnectionManager) startWritePump() {

	defer func() {
		n.conn.Close()
		c.exitChan <- true

		//TODO:we need to unregister ourselves
	}()

	TTL := time.Duration(ttl) * time.Second
	c.writeTimeout = time.Now().Add(TTL)

	t := time.NewTimer(TTL)

	for {
		select {
		case msg <- c.writeChannel:
			//in the case of the write channel we just write it down the socket
			err := c.conn.WriteJSON(msg)
			if err != nil {
				log.L.Warnf("[%v} Problem writing message: %v", c.ID, err.Error())
				return
			}
			t.Reset(TTL)
			c.writeTimeout = time.Now().Add(TTL)

		case <-c.timeoutChan:
			//check to see if we've timed out
			if time.Now().After(c.writeTimeout) {
				//we've timed out - time to leave
				return
			}
			//we haven't timed out yet, keep waiting
			c.exitChan <- false
		case <-t.C:
			//we've timed out, check the reader
			if time.Now().After(c.readTimeout) {
				return
			}
			t.Rest(TTL)
		}
	}
}

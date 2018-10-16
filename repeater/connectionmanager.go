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

var (
	//TTL is TimeToLive
	TTL = time.Duration(ttl) * time.Second
)

//ConnectionManager .
type ConnectionManager struct {
	conn websocket.Conn

	ID   string
	Room string

	//internal channels
	readChannel  chan event.Event
	writeChannel chan event.Event

	readExit  chan bool
	writeExit chan bool
	errorChan chan error

	writeTimeout time.Time
	readTimeout  time.Time

	//external channels
	ReceiveChannel chan event.Event
	SendChannel    chan event.Event
}

//We don't try to re-establish this one, nor do we worry about ping/pong joy - we're alive until one of us closes it - hopefully 5 seconds of inactivity
func (c *ConnectionManager) startReadPump() {

	c.conn.SetReadDeadline(time.Now().Add(TTL))
	for {
		var event events.Event
		t, b, err := c.conn.ReadJSON(&event)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway) {
				log.L.Errorf("[%v] Websocket closing: %v", c.ID, err)
			} else {
				netErr, ok := err.(net.Error)
				if ok && netErr.Timeout() {
					select {
					case <-readExit:
						return
					default:
						c.conn.SetReadDeadline(time.Now().Add(TTL))
						continue
					}
				}
			}
			log.L.Debugf("[%v] Returning", c.ID, err)
			c.errorChan <- err
			return
		}

		c.readChannel <- m

		c.conn.SetReadDeadline(time.Now().Add(TTL))
	}
}

func (c *ConnectionManager) startWritePump() {

	c.conn.SetWriteDeadline = time.Now().Add(TTL)

	for {
		select {
		case msg <- c.writeChannel:
			//in the case of the write channel we just write it down the socket
			err := c.conn.WriteJSON(msg)
			if err != nil {
				log.L.Warnf("[%v} Problem writing message: %v", c.ID, err.Error())
				c.errorChan <- err
				return
			}
			c.conn.SetWriteDeadline = time.Now().Add(TTL)

		case <-c.writeExit:
			return
		}
	}
}

func (c *ConnectionManager) startPumper() {
	defer func() {
		//TODO: deregister

		c.writeExit <- true
		c.readExit <- true

		time.Sleep(TTL)
		c.conn.Close()
	}()

	c.readTimeout = time.Now().Add(TTL)
	c.writeTimeout = time.Now().Add(TTL)

	//start our ticker
	t := time.NewTicker(TTL)
	select {
	case <-t.C:
		//check to see if read and write are after now
		if time.Now().After(c.readTimeout) && time.Now().After(c.writeTimeout) {
			//time to leave
			return
		}

	case err := <-c.errorChan:
		//there was an error
		log.L.Infof("[%v] error: %v. Closing..", c.ID, err.Error())
		return

	case e := <-c.SendChannel:
		c.writeTimeout = time.Now().Add(TTL)
		c.writeChannel <- e

	case e := <-c.readChannel:
		c.readTimeout = time.Now().Add(TTL)
		c.ReceiveChannel <- e
	}

}

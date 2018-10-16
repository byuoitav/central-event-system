package repeater

import (
	"net/http"
	"sync"

	"github.com/byuoitav/central-event-system/hub/base"
	"github.com/byuoitav/central-event-system/messenger"
	"github.com/byuoitav/common/log"
	"github.com/byuoitav/common/nerr"
	"github.com/byuoitav/common/v2/events"
	"github.com/labstack/echo"
)

//Repeater .
type Repeater struct {
	messenger      messenger.Messenger
	connectionLock sync.RWMutex
	connections    map[string]*PumpingStation //map of IDs to ConnectionManager

	sendMap     map[string][]string //for future use, map of affected rooms to the control processors that care about that room
	sendMapLock sync.RWMutex

	HubSendBuffer chan events.Event
}

//GetRepeater .
func GetRepeater(s map[string][]string, m messenger.Messenger) *Repeater {
	v := &Repeater{
		sendMap:        s,
		HubSendBuffer:  make(chan events.Event, 1000),
		sendMapLock:    sync.RWMutex{},
		connectionLock: sync.RWMutex{},
		connections:    make(map[string]*PumpingStation),
		messenger:      m,
	}
	go v.runRepeater()

	return v
}

//RunRepeaterTranslator will take an event and format it in the proper way to translate to the hub format
func (r *Repeater) runRepeaterTranslator() *nerr.E {
	var e events.Event
	for {
		e = <-HubSendBuffer
		r.messenger.SendEvent(base.WrapEvent(e))
	}
}

//RunRepeater .
func (r *Repeater) runRepeater() {

	go RunRepeaterTranslator()

	var e events.Event
	var addrs []string
	for {
		e = base.UnwrapEvent(r.Messenger.ReceiveEvent())

		//Get the list of places we're sending it
		sendMapLock.RLock()
		conns = sendMap[e.AffectedRoom.RoomID]
		sendMapLock.RUnlock()

		for a := range conns {
			connectionLock.RLock()
			v, ok := connections[conns[a]]
			connectionLock.RUnlock()

			if ok {
				v.SendEvent(e)
			} else {
				//we need to start a connection, register it, and then send this connection down that channel
				log.L.Infof("Sending event to %v, need to start a connection...", conns[a])
				p, err := StartConnection(conns[a], e.AffectedRoom.RoomID, r)
				if err != nil {
					log.Errorf("Couldn't start connection with %v", conns[a])
					continue
				}

				p.SendEvent(e)

				connectionLock.Lock()
				connections[conns[a]] = p
				connectionLock.Unlock()

			}
		}
	}
}

func (r *Repeater) handleConnection(context echo.Context) error {
	id := context.Param("id")
	room := context.Param("room")

	conn, err := upgrader.Upgrade(context.Response().Writer, context.Request(), nil)
	if err != nil {
		log.L.Errorf("Couldn't upgrade	Connection to a websocket: %v", err.Error())
		return err
	}
	p, er := buildFromConnection(id, room, r, conn)
	if er != nil {
		return context.JSON(http.StatusBadRequest, er.Error())
	}
	RegisterConnection(p)

	//so we bock
	p.startPumper()

	return nil
}

//RegisterConnection .
func (r *Repeater) RegisterConnection(c *PumpingStation) {
	log.L.Infof("registering connection to %v", c.ID)
	//check to see if it's a duplicate id
	connectionLock.RLock()
	_, ok := connections[c.ID]
	connectionLock.RUnlock()

	cur := 0
	for ok {
		cur++
		//we need to change the id
		log.L.Warnf("Duplicate connection: %v, creating connection %v", c.ID, cur)
		c.ID = fmr.Sprintf("%v:%v", c.ID, cur)

		connectionLock.RLock()
		_, ok := connections[c.ID]
		connectionLock.RUnlock()
	}

	connectionLock.Lock()
	connections[c.ID] = c
	connectionLock.Unlock()

	log.L.Infof("connection to %v added", c.ID)
}

//UnregisterConnection .
func (r *Repeater) UnregisterConnection(id string) {
	log.L.Infof("Removing registration connection to %v", id)
	connectionLock.Lock()
	delete(connections, id)
	connectionLock.Unlock()
	log.L.Infof("Done removing registration for %v", id)
}

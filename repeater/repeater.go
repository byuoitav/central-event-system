package main

import (
	"fmt"
	"net/http"
	"os"
	"sync"

	"github.com/byuoitav/central-event-system/hub/base"
	"github.com/byuoitav/central-event-system/messenger"
	"github.com/byuoitav/common/log"
	"github.com/byuoitav/common/nerr"
	"github.com/byuoitav/common/v2/events"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo"
)

//Repeater .
type Repeater struct {
	messenger      *messenger.Messenger
	connectionLock sync.RWMutex
	connections    map[string]*PumpingStation //map of IDs to ConnectionManager

	sendMap     map[string][]string //for future use, map of affected rooms to the control processors that care about that room
	sendMapLock sync.RWMutex

	HubSendBuffer chan events.Event
}

var (
	//HubAddress for the repeater to connect to.
	HubAddress string

	//SendMap is the base map for what rooms send to what pis
	SendMap map[string][]string

	upgrader = websocket.Upgrader{
		ReadBufferSize:  2048,
		WriteBufferSize: 2048,
	}
)

func init() {
	HubAddress = os.Getenv("HUB_ADDRESS")
	if len(HubAddress) < 1 {
		log.L.Infof("No hub address specified, default to localhost:7100")
		HubAddress = "localhost:7100"
	}

	SendMap := make(map[string][]string)
	SendMap["ITB-1101"] = []string{"ITB-1101-CP8"}
}

//GetRepeater .
func GetRepeater(s map[string][]string, m *messenger.Messenger) *Repeater {
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
		e = <-r.HubSendBuffer
		r.messenger.SendEvent(base.WrapEvent(e))
	}
}

//RunRepeater .
func (r *Repeater) runRepeater() {

	go r.runRepeaterTranslator()

	var e events.Event
	var err *nerr.E
	var conns []string
	for {
		msg := r.messenger.ReceiveEvent()
		e, err = base.UnwrapEvent(msg)
		if err != nil {
			log.L.Warnf("Couldn't unwrap event: %v", msg)
			continue
		}

		//Get the list of places we're sending it
		r.sendMapLock.RLock()
		conns = r.sendMap[e.AffectedRoom.RoomID]
		r.sendMapLock.RUnlock()

		for a := range conns {
			r.connectionLock.RLock()
			v, ok := r.connections[conns[a]]
			r.connectionLock.RUnlock()

			if ok {
				v.SendEvent(e)
			} else {
				//we need to start a connection, register it, and then send this connection down that channel
				log.L.Infof("Sending event to %v, need to start a connection...", conns[a])
				p, err := StartConnection(conns[a], e.AffectedRoom.RoomID, r)
				if err != nil {
					log.L.Errorf("Couldn't start connection with %v", conns[a])
					continue
				}

				p.SendEvent(e)

				r.connectionLock.Lock()
				r.connections[conns[a]] = p
				r.connectionLock.Unlock()

			}
		}
	}
}

func (r *Repeater) fireEvent(context echo.Context) error {
	var e events.Event
	err := context.Bind(&e)
	if err != nil {
		return context.String(http.StatusBadRequest, fmt.Sprintf("Invalid request, must send an event. Error: %v", err.Error()))
	}
	r.HubSendBuffer <- e

	return context.String(http.StatusOK, "ok")
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
	r.RegisterConnection(p)

	//so we bock
	p.startPumper()

	return nil
}

//RegisterConnection .
func (r *Repeater) RegisterConnection(c *PumpingStation) {
	log.L.Infof("registering connection to %v", c.ID)
	//check to see if it's a duplicate id
	r.connectionLock.RLock()
	_, ok := r.connections[c.ID]
	r.connectionLock.RUnlock()

	cur := 0
	for ok {
		cur++
		//we need to change the id
		log.L.Warnf("Duplicate connection: %v, creating connection %v", c.ID, cur)
		c.ID = fmt.Sprintf("%v:%v", c.ID, cur)

		r.connectionLock.RLock()
		_, ok = r.connections[c.ID]
		r.connectionLock.RUnlock()
	}

	r.connectionLock.Lock()
	r.connections[c.ID] = c
	r.connectionLock.Unlock()

	log.L.Infof("connection to %v added", c.ID)
}

//UnregisterConnection .
func (r *Repeater) UnregisterConnection(id string) {
	log.L.Infof("Removing registration connection to %v", id)

	r.connectionLock.Lock()
	delete(r.connections, id)
	r.connectionLock.Unlock()

	log.L.Infof("Done removing registration for %v", id)
}

package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/byuoitav/central-event-system/hub/base"
	"github.com/byuoitav/central-event-system/messenger"
	"github.com/byuoitav/central-event-system/repeater/httpbuffer"
	"github.com/byuoitav/common/log"
	"github.com/byuoitav/common/nerr"
	"github.com/byuoitav/common/status"
	"github.com/byuoitav/common/v2/events"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo"
)

// Repeater .
type Repeater struct {
	messenger      *messenger.Messenger
	connectionLock sync.RWMutex
	connections    map[string]*PumpingStation //map of IDs to ConnectionManager

	sendMap     map[string][]string //for future use, map of affected rooms to the control processors that care about that room
	sendMapLock sync.RWMutex

	httpBuffer *httpbuffer.HTTPBuffer
	httpAddrs  []string

	HubSendBuffer chan base.EventWrapper
	RepeaterID    string
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
	//log.SetLevel("debug")

	HubAddress = os.Getenv("HUB_ADDRESS")
	if len(HubAddress) < 1 {
		log.L.Infof("No hub address specified, default to ws://localhost:7100")
		HubAddress = "ws://localhost:7100"
	}
	var err *nerr.E

	SendMap, err = BuildSendList()
	for k, v := range SendMap {
		log.L.Debugf("%v: %v", k, v)
	}
	if err != nil {
		log.L.Fatalf("Couldn't initialize the repeater: %v", err.Error())
	}
}

// GetRepeater .
func GetRepeater(s map[string][]string, m *messenger.Messenger, id string) *Repeater {
	v := &Repeater{
		sendMap:        s,
		HubSendBuffer:  make(chan base.EventWrapper, 1000),
		sendMapLock:    sync.RWMutex{},
		connectionLock: sync.RWMutex{},
		connections:    make(map[string]*PumpingStation),
		messenger:      m,
		RepeaterID:     id,
		httpBuffer:     httpbuffer.New(2*time.Second, 1000),
	}

	go v.runRepeater()

	return v
}

// RunRepeaterTranslator will take an event and format it in the proper way to translate to the hub format
func (r *Repeater) runRepeaterTranslator() *nerr.E {
	var e base.EventWrapper
	for {
		e = <-r.HubSendBuffer
		r.messenger.Send(e)
	}
}

func (r *Repeater) runRepeater() {
	log.L.Infof("Running repeater...")

	go r.runRepeaterTranslator()

	var msg base.EventWrapper
	var conns []string
	var starconns []string
	for {
		msg = r.messenger.Receive()
		log.L.Debugf("Distributing  an event for %v", msg.Room)

		//Get the list of places we're sending it
		r.sendMapLock.RLock()
		conns = r.sendMap[msg.Room]
		starconns = r.sendMap["*"]
		r.sendMapLock.RUnlock()

		for a := range conns {
			//check if it's an http/s endpoint
			if strings.HasPrefix(conns[a], "http") {
				//we just package it up and send it out
				r.httpBuffer.SendEvent(msg.Event, "POST", conns[a])
				continue
			}

			r.connectionLock.RLock()
			v, ok := r.connections[conns[a]]
			r.connectionLock.RUnlock()

			if ok {
				v.SendEvent(msg)
			} else {
				//we need to start a connection, register it, and then send this connection down that channel
				log.L.Infof("Sending event to %v, need to start a connection...", conns[a])
				p, err := StartConnection(conns[a], msg.Room, r, true)
				if err != nil {
					log.L.Errorf("Couldn't start connection with %v", conns[a])
					continue
				}

				p.SendEvent(msg)

				r.connectionLock.Lock()
				r.connections[conns[a]] = p
				r.connectionLock.Unlock()

			}
		}
		for a := range starconns {

			//check if it's an http/s endpoint
			if strings.HasPrefix(starconns[a], "http") {
				//we just package it up and send it out
				r.httpBuffer.SendEvent(msg.Event, "POST", starconns[a])
				continue
			}

			r.connectionLock.RLock()
			v, ok := r.connections[starconns[a]]
			r.connectionLock.RUnlock()

			if ok {
				v.SendEvent(msg)
			} else {
				//we need to start a connection, register it, and then send this connection down that channel
				log.L.Infof("Sending event to %v, need to start a connection...", starconns[a])
				p, err := StartConnection(starconns[a], msg.Room, r, false)
				if err != nil {
					log.L.Errorf("Couldn't start connection with %v", starconns[a])
					continue
				}

				p.SendEvent(msg)

				r.connectionLock.Lock()
				r.connections[starconns[a]] = p
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
	r.HubSendBuffer <- base.WrapEvent(e)

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

// RegisterConnection .
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

	log.L.Debugf("connection to %v added", c.ID)
}

// UnregisterConnection .
func (r *Repeater) UnregisterConnection(id string) {
	log.L.Infof("[%v] Removing registration connection.", id)

	r.connectionLock.Lock()
	delete(r.connections, id)
	r.connectionLock.Unlock()

	log.L.Debugf("Done removing registration for %v", id)
}

// Status .
type Status struct {
	Connections []PumpingStationStatus `json:"pumping-stations"`
	HTTPStatus  httpbuffer.Status      `json:"http-buffer"`
	Hub         interface{}            `json:"hub-status"`
}

// GetStatus .
func (r *Repeater) GetStatus(context echo.Context) error {
	st := status.NewBaseStatus()
	s := Status{
		Hub:        r.messenger.GetState(),
		HTTPStatus: r.httpBuffer.GetStatus(),
	}
	r.connectionLock.RLock()
	for i := range r.connections {
		s.Connections = append(s.Connections, r.connections[i].GetStatus())
	}
	r.connectionLock.RUnlock()

	st.Info["repeater"] = s

	return context.JSON(http.StatusOK, st)
}

package main

import (
	"net/http"

	"github.com/byuoitav/central-event-system/hub/base"
	"github.com/byuoitav/central-event-system/messenger"
	"github.com/byuoitav/common"
	"github.com/byuoitav/common/log"
	"github.com/byuoitav/common/nerr"
	"github.com/byuoitav/common/v2/events"
	"github.com/labstack/echo"
)

var m1 *messenger.Messenger
var m2 *messenger.Messenger

func main() {
	HubAddress := "localhost:7100"
	var err *nerr.E

	m1, err = messenger.BuildMessenger(HubAddress, base.Messenger, 1000)
	if err != nil {
		if err.Type == "retrying" {
			log.L.Warnf("Retrying connection to hub")
		} else {
			log.L.Fatalf("Couldn't build messenger: %v", err.Error())
		}
	}
	m2, err = messenger.BuildMessenger(HubAddress, base.Messenger, 1000)
	if err != nil {
		if err.Type == "retrying" {
			log.L.Warnf("Retrying connection to hub")
		} else {
			log.L.Fatalf("Couldn't build messenger: %v", err.Error())
		}
	}
	m1.SubscribeToRooms([]string{"ITB-M2", "ITB-M1", "ITB-1101"})
	m1.SubscribeToRooms([]string{"ITB-M2"})
	go func() {
		a := m1.ReceiveEvent()
		log.L.Infof("m1 Got event for %v", a.AffectedRoom.RoomID)
	}()
	go func() {
		a := m2.ReceiveEvent()
		log.L.Infof("m2 Got event for %v", a.AffectedRoom.RoomID)
	}()

	r := common.NewRouter()
	r.POST("/1", func(context echo.Context) error {
		return sendEvent(context, m1, "ITB-M1")
	})
	r.POST("/2", func(context echo.Context) error {
		return sendEvent(context, m2, "ITB-M2")
	})

	r.POST("/sub/:id/:room", subscribe)
	r.POST("/usub/:id/:room", unsubscribe)
	r.Start(":7015")

}

func subscribe(context echo.Context) error {
	id := context.Param("id")
	room := context.Param("room")
	log.L.Infof("Subscribting %v to %v", id, room)
	if id == "1" {
		m1.SubscribeToRooms([]string{room})
	} else {
		m2.SubscribeToRooms([]string{room})
	}

	return context.String(http.StatusOK, "ok")
}

func unsubscribe(context echo.Context) error {
	id := context.Param("id")
	room := context.Param("room")
	log.L.Infof("Unsubscribing %v to %v", id, room)
	if id == "1" {
		m1.UnsubscribeFromRooms([]string{room})
	} else {
		m2.UnsubscribeFromRooms([]string{room})
	}

	return context.String(http.StatusOK, "ok")
}

func sendEvent(context echo.Context, m *messenger.Messenger, room string) error {
	var a events.Event
	err := context.Bind(&a)
	if err != nil {
		log.L.Warnf("Bad request")
		return context.String(http.StatusBadRequest, err.Error())
	}

	log.L.Infof("sending event")

	m.SendEvent(a)

	return context.String(http.StatusOK, "ok")
}

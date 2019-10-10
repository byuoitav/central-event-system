package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/byuoitav/central-event-system/hub/base"
	"github.com/byuoitav/central-event-system/hub/hubconn"
	"github.com/byuoitav/central-event-system/hub/nexus"
	"github.com/byuoitav/common"
	"github.com/byuoitav/common/log"
	"github.com/byuoitav/common/status"
	"github.com/byuoitav/common/v2/events"
	"github.com/labstack/echo"
)

func main() {
	port := ":7100"

	nexus.StartNexus()

	// if this hub is in a room, create an interconnection with the rest of the hubs in the room
	if len(os.Getenv("ROOM_SYSTEM")) > 0 {
		addresses := GetHubAddresses()

		for i := range addresses {
			log.L.Infof("Opening hub interconnection with %v", addresses[i])
			go hubconn.OpenConnectionWithRetry(addresses[i], "/connect/hub", base.Hub, nexus.N)
		}
	}

	router := common.NewRouter()

	router.GET("/status", Status)
	router.GET("/connect/:type", func(context echo.Context) error {
		t := context.Param("type")
		switch t {
		case base.Messenger, base.Repeater, base.Hub:
			break
		default:
			return context.String(http.StatusBadRequest, "invalid connection type")
		}

		err := hubconn.CreateConnection(context.Response().Writer, context.Request(), t, nexus.N)
		if err != nil {
			return context.JSON(http.StatusInternalServerError, err.Error())
		}

		return nil
	})

	router.POST("/interconnect/:address", func(context echo.Context) error {
		return CreateInterconnection(context, nexus.N)
	})

	router.POST("/event", Event)

	router.Start(port)
}

// Status returns the status of the hub
func Status(ctx echo.Context) error {
	log.L.Debugf("Status request from %v", ctx.Request().RemoteAddr)

	var s status.Status
	var err error

	s.Info = map[string]interface{}{}
	s.Bin = os.Args[0]
	s.Info = make(map[string]interface{})
	s.Uptime = status.GetProgramUptime().String()

	s.Version, err = status.GetMicroserviceVersion()
	if err != nil {
		s.Info["error"] = "failed to open version.txt"
		s.StatusCode = status.Sick

		return ctx.JSON(http.StatusInternalServerError, s)
	}

	s.Info["nexus"] = nexus.N.GetStatus()
	s.StatusCode = status.Healthy

	return ctx.JSON(http.StatusOK, s)
}

// Event sends an event to the hub using an http endpoint instead of a messenger
func Event(c echo.Context) error {
	var e events.Event
	req := c.Request()

	eventBytes, err := ioutil.ReadAll(req.Body)
	if err != nil {
		log.L.Warnf("unable to read body: " + err.Error())
		return c.String(http.StatusBadRequest, "unable to read body: "+err.Error())
	}

	log.L.Debugf("Submitting event from %s: %s", c.Request().RemoteAddr, eventBytes)

	err = json.Unmarshal(eventBytes, &e)
	if err != nil {
		log.L.Warnf("unable to unmarshal body: " + err.Error())
		return c.String(http.StatusBadRequest, "unable to unmarshal body: "+err.Error())
	}

	nerr := nexus.N.Submit(
		base.EventWrapper{
			Room:  e.AffectedRoom.RoomID,
			Event: eventBytes,
		},
		base.Messenger,
		req.RemoteAddr+base.Messenger)
	if nerr != nil {
		log.L.Warnf("unable to submit event: " + nerr.Error())
		return c.String(http.StatusInternalServerError, "unable to submit event: "+nerr.Error())
	}

	return c.String(http.StatusOK, "Processing event")
}

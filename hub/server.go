package main

import (
	"net/http"

	"github.com/byuoitav/central-event-system/hub/base"
	"github.com/byuoitav/central-event-system/hub/incomingconnection"
	"github.com/byuoitav/central-event-system/hub/nexus"
	"github.com/byuoitav/common"
	"github.com/labstack/echo"
)

func main() {
	port := ":7100"

	router := common.NewRouter()
	router.GET("/connect/:type", func(context echo.Context) error {
		t := context.Param("type")
		switch t {
		case base.Messenger, base.Broadcaster, base.Receiver, base.Hub:
			break
		default:
			return context.JSON(http.StatusBadRequest, "Invalid connection type")
		}
		err := incomingconnection.CreateConnection(context.Response().Writer, context.Request(), base.Messenger, nexus.N)
		if err != nil {
			return context.JSON(http.StatusInternalServerError, err.Error())
		}
		return nil
	})

	router.Start(port)
}

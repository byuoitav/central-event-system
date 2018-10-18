package main

import (
	"net/http"
	"os"

	"github.com/byuoitav/central-event-system/hub/base"
	"github.com/byuoitav/central-event-system/hub/incomingconnection"
	"github.com/byuoitav/central-event-system/hub/nexus"
	"github.com/byuoitav/common"
	"github.com/byuoitav/common/log"
	"github.com/byuoitav/common/status"
	"github.com/labstack/echo"
)

func main() {
	log.SetLevel("debug")
	port := ":7100"

	nexus.StartNexus()

	router := common.NewRouter()
	router.GET("/connect/:type", func(context echo.Context) error {
		t := context.Param("type")
		switch t {
		case base.Messenger, base.Repeater, base.Hub:
			break
		default:
			return context.JSON(http.StatusBadRequest, "Invalid connection type")
		}
		err := incomingconnection.CreateConnection(context.Response().Writer, context.Request(), t, nexus.N)
		if err != nil {
			return context.JSON(http.StatusInternalServerError, err.Error())
		}
		return nil
	})

	router.GET("/mstatus", mstatus)
	router.Start(port)
}

func mstatus(ctx echo.Context) error {
	log.L.Infof("MStatus request from %v", ctx.Request().RemoteAddr)

	var s status.MStatus
	var err error

	s.Bin = os.Args[0]

	s.Version, err = status.GetMicroserviceVersion()
	if err != nil {
		s.Info = "failed to open version.txt"
		s.StatusCode = status.Sick

		return ctx.JSON(http.StatusInternalServerError, s)
	}
	s.Info = nexus.N.GetStatus()

	s.StatusCode = status.Healthy

	return ctx.JSON(http.StatusOK, s)

}

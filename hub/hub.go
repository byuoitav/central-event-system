package main

import (
	"fmt"
	"net/http"

	"github.com/byuoitav/central-event-system/hub/base"
	"github.com/byuoitav/central-event-system/hub/hubconn"
	"github.com/byuoitav/central-event-system/hub/nexus"
	"github.com/labstack/echo"
)

//CreateInterconnection .
func CreateInterconnection(context echo.Context, n *nexus.Nexus) error {
	hubaddr := context.Param("address")
	err := hubconn.OpenConnection(hubaddr+":7100", "/connect/hub", base.Hub, n)
	if err != nil {
		return context.String(http.StatusInternalServerError, fmt.Sprintf("Couldn't esatblish hub connction: %v", err.Error()))
	}

	return context.String(http.StatusOK, "ok")
}

package main

import (
	"os"

	"github.com/byuoitav/central-event-system/hub/base"
	"github.com/byuoitav/central-event-system/messenger"
	"github.com/byuoitav/common"
	"github.com/byuoitav/common/log"
)

func main() {
	port := ":7101"
	m, err := messenger.BuildMessenger(HubAddress, base.Repeater, 1000)
	if err != nil {
		if err.Type == "retrying" {
			log.L.Warnf("Retrying connection to hub")
		} else {
			log.L.Fatalf("Couldn't build messenger: %v", err.Error())
		}
	}

	//do we want some sort of config here?
	r := GetRepeater(SendMap, m, os.Getenv("SYSTEM_ID"))

	router := common.NewRouter()

	router.GET("/status", r.GetStatus)
	router.GET("/connect/:room/:id", r.handleConnection)
	router.POST("send", r.fireEvent)

	router.Start(port)
}

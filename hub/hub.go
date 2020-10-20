package main

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/byuoitav/central-event-system/hub/base"
	"github.com/byuoitav/central-event-system/hub/hubconn"
	"github.com/byuoitav/central-event-system/hub/nexus"
	"github.com/byuoitav/common/db"
	"github.com/byuoitav/common/log"
	"github.com/byuoitav/common/v2/events"
	"github.com/labstack/echo"
)

// TODO put port into a const
var dev sync.Once

//CreateInterconnection . fyi this will NOT work :)
func CreateInterconnection(context echo.Context, n *nexus.Nexus) error {
	hubaddr := context.Param("address")
	err := hubconn.OpenConnection(hubaddr+":7100", "/connect/hub", base.Hub, n, false)
	if err != nil {
		return context.String(http.StatusInternalServerError, fmt.Sprintf("Couldn't establish hub connction: %v", err.Error()))
	}

	return context.String(http.StatusOK, "ok")
}

// GetHubAddresses returns a list of hubs this hub should try to connect to.
func GetHubAddresses() []string {
	log.L.Infof("Getting list of hubs I should connect to")
	addresses := []string{}

	id := os.Getenv("SYSTEM_ID")
	roomID := events.GenerateBasicDeviceInfo(id).RoomID

	regexStr := `[a-zA-z]+(\d+)$`
	re := regexp.MustCompile(regexStr)
	matches := re.FindAllStringSubmatch(id, -1)
	if len(matches) != 1 {
		log.L.Infof("Event router limited to only Control Processors.")
		return nil
	}

	myNum, _ := strconv.Atoi(matches[0][1])
	log.L.Debugf("My processor number: %v", myNum)

	for {
		devices, err := db.GetDB().GetDevicesByRoomAndRole(roomID, "EventRouter")
		if err != nil {
			log.L.Warnf("unable to get devices in %s: %s", roomID, err.Error())
			time.Sleep(5 * time.Second)
			continue
		}

		if len(devices) == 0 {
			log.L.Warnf("No devices found in room. Couch may not have replicated completly yet, or this device may have the wrong hostname.")
			time.Sleep(5 * time.Second)
			continue
		}

		for _, device := range devices {
			if len(os.Getenv("DEV_HUB")) > 0 {
				dev.Do(func() {
					log.L.Infof("Development device. Adding all hubs in room")
				})

				addresses = append(addresses, "ws://"+device.Address+":7100")
				continue
			}

			// skip it if this one is me
			if strings.EqualFold(device.ID, id) {
				continue
			}

			log.L.Debugf("Considering device: %v", device.ID)
			matches = re.FindAllStringSubmatch(device.Name, -1)
			if len(matches) != 1 {
				continue
			}

			num, err := strconv.Atoi(matches[0][1])
			if err != nil {
				continue
			}

			if num < myNum {
				continue
			}

			log.L.Debugf("Adding hub %v to address list.", device.Address)
			addresses = append(addresses, "ws://"+device.Address+":7100")
		}

		break
	}

	log.L.Infof("Done. Found %v routers", len(addresses))
	return addresses
}

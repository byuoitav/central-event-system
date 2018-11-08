package main

import (
	"os"
	"strings"
	"time"

	"github.com/byuoitav/common/db"
	"github.com/byuoitav/common/log"
	"github.com/byuoitav/common/nerr"
	"github.com/byuoitav/common/v2/events"
)

//BuildSendList .
func BuildSendList() (map[string][]string, *nerr.E) {
	log.L.Infof("building the send list")

	toReturn := map[string][]string{}
	//get my room, if I have one
	RoomSystem := len(os.Getenv("ROOM_SYSTEM")) > 0
	roomID := ""

	if RoomSystem {
		//we need to get the room info
		roomID = events.GenerateBasicDeviceInfo(os.Getenv("SYSTEM_ID")).RoomID
	}

	//wait for the database to be ready
	// +deploy not-required
	for len(os.Getenv("ROOM_SYSTEM")) > 0 && len(os.Getenv("STOP_REPLICATION")) < 1 {
		state, err := db.GetDB().GetStatus()
		if err != nil || state != "completed" {
			log.L.Infof("Database replication in state %v; Retrying in 5 seconds", state)
			time.Sleep(5 * time.Second)
			continue
		} else {
			log.L.Infof("Database ready, building the send list")
			break
		}
	}
	log.L.Debugf("Getting all the event hubs from the databsae")

	//We need to get all the devices with the role of repeater from the database - then figure out what rooms they care about, for now we just assume that it's their containing room.
	devs, err := db.GetDB().GetDevicesByRoleAndType("EventRouter", "Pi3")
	if err != nil {
		return toReturn, nerr.Translate(err).Addf("Couldn't build send list")
	}

	log.L.Debugf("Got %v devices.", len(devs))

	for i := range devs {
		split := strings.Split(devs[i].ID, "-")
		rm := split[0] + "-" + split[1]
		if RoomSystem && rm == roomID {
			//its in our room, ignore
			continue
		}
		//we add it
		toReturn[rm] = append(toReturn[rm], devs[i].ID)
	}

	//if it's an in room system I know I need to send it to the central hub - so we'll add that as a '*'.
	if len(os.Getenv("CENTRAL_REPEATER_ADDRESS")) < 1 {
		log.L.Infof("CENTRAL_REPEATER_ADDRESS not set, event will not be sent to a central hub")
	} else {
		toReturn["*"] = []string{os.Getenv("CENTRAL_REPEATER_ADDRESS")}
	}

	log.L.Infof("Done, send list built for %v rooms.", len(toReturn))
	return toReturn, nil
}

package nexus

import "github.com/byuoitav/central-event-system/hub/base"

//RegStatus represents the status of a registration
type RegStatus struct {
	ID         string `json:"id"`
	BufferCap  int    `json:"buffer-capacity"`
	BufferUtil int    `json:"buffer-utilization"`
}

//Status returns the state of the nexus, including the states of the registries, and the utilization of the buffers.
type Status struct {
	Hubs              []RegStatus            `json:"hubs"`
	Messengers        []string               `json:"messengers"`
	Repeaters         []RegStatus            `json:"repeaters"`
	MessengerMappings map[string][]RegStatus `json:"messenger-mapping"`
	Registration      RegStatus              `json:"registration-buffer"`
	Distribution      RegStatus              `json:"distribution-buffer"`
}

//GetStatus returns the state of the device
func (n *Nexus) GetStatus() Status {
	toReturn := Status{
		MessengerMappings: make(map[string][]RegStatus),
		Hubs:              []RegStatus{},
		Repeaters:         []RegStatus{},
		Messengers:        []string{},
	}

	tmpmessengers := make(map[string]bool)

	//get the length and capacity of the
	toReturn.Registration = RegStatus{
		ID:         "registration",
		BufferCap:  cap(n.registrationChannel),
		BufferUtil: len(n.registrationChannel),
	}
	toReturn.Distribution = RegStatus{
		ID:         "distribution",
		BufferCap:  cap(n.incomingChannel),
		BufferUtil: len(n.incomingChannel),
	}

	for k, v := range n.messengerRegistry {
		cur := []RegStatus{}
		for i := range v {
			cur = append(cur, RegStatus{
				ID:         v[i].ID,
				BufferCap:  cap(v[i].Channel),
				BufferUtil: len(v[i].Channel),
			})
			tmpmessengers[v[i].ID] = true
		}
		toReturn.MessengerMappings[k] = cur
	}

	toReturn.Hubs = getRegistryStatus(n.hubRegistry)
	toReturn.Repeaters = getRegistryStatus(n.repeaterRegistry)

	return toReturn
}

func getRegistryStatus(v []base.Registration) []RegStatus {
	toReturn := []RegStatus{}
	for i := range v {
		toReturn = append(toReturn, RegStatus{
			ID:         v[i].ID,
			BufferCap:  cap(v[i].Channel),
			BufferUtil: len(v[i].Channel),
		})
	}
	return toReturn
}

package nexus

import (
	"os"
	"sync"

	"github.com/byuoitav/central-event-system/hub/base"
	"github.com/byuoitav/common/log"
	"github.com/byuoitav/common/nerr"
)

//N is the default Nexus
//Do this in server.go?
var N *Nexus

//StartNexus .
func StartNexus() {

	log.L.Infof("Warping in nexus...")
	N = &Nexus{
		registrationChannel: make(chan base.RegistrationChange, 100),
		incomingChannel:     make(chan base.HubEventWrapper, 5000),

		messengerRegistry:  make(map[string][]base.Registration),
		roomMessengerIndex: make(map[string][]string),
		roomNexus:          len(os.Getenv("ROOM_SYSTEM")) > 0,
	}
	//start the router
	go N.start()
	log.L.Infof("Done.")
}

//Nexus handles the actuall routing of events around
type Nexus struct {
	messengerRegistry  map[string][]base.Registration
	roomMessengerIndex map[string][]string

	hubRegistry      []base.Registration
	repeaterRegistry []base.Registration

	registrationChannel chan base.RegistrationChange
	incomingChannel     chan base.HubEventWrapper

	roomNexus bool

	once sync.Once
}

//SubmitRegistrationChange .
func (n *Nexus) SubmitRegistrationChange(r base.RegistrationChange) {
	n.registrationChannel <- r
}

//RegisterConnection in cases of spokes is called with a set of rooms, and the channel to send events for that room down. in cases of dispatchers and hubs the rooms array is ignored.
func (n *Nexus) RegisterConnection(rooms []string, channel chan base.EventWrapper, connID, connType string) *nerr.E {
	log.L.Debugf("Registring connection %v of type %v for rooms %v", connID, connType, rooms)
	n.registrationChannel <- base.RegistrationChange{
		Type: connType,
		SubscriptionChange: base.SubscriptionChange{
			Create: true,
			Rooms:  rooms,
		},
		Registration: base.Registration{
			Channel: channel,
			ID:      connID,
		},
	}
	return nil
}

//DeregisterConnection will deregsiter the provided connection (type + ID) fro all rooms provided. In cases of dispatchers and hubs the rooms parameter is ignored
func (n *Nexus) DeregisterConnection(rooms []string, connType, connID string) *nerr.E {

	n.registrationChannel <- base.RegistrationChange{
		Type: connType,
		Registration: base.Registration{
			ID: connID,
		},
		SubscriptionChange: base.SubscriptionChange{
			Create: false,
			Rooms:  rooms,
		},
	}
	return nil
}

//Submit sends an event to the hub for routing
func (n *Nexus) Submit(e base.EventWrapper, Source, SourceID string) *nerr.E {

	if len(Source) == 0 || len(SourceID) == 0 {
		return nerr.Create("Can't submit blank source or sourceID", "invalid")
	}

	n.incomingChannel <- base.HubEventWrapper{
		Source:       Source,
		SourceID:     SourceID,
		EventWrapper: e,
	}

	return nil
}

func (n *Nexus) start() {
	curRepeater := 0
	n.once.Do(func() {
		for {
			select {
			case e := <-n.incomingChannel:
				log.L.Debugf("Sending Event from %v of type %v for room %v", e.SourceID, e.Source, e.Room)
				if v, ok := n.messengerRegistry[e.Room]; ok {
					//we ALWAYS send to spokes
					for i := range v {
						if e.Source != base.Messenger || v[i].ID != e.SourceID {
							log.L.Debugf("%v", v[i].ID)
							v[i].Channel <- e.EventWrapper
						}
					}
				}
				//where else do we send it?
				switch e.Source {
				case base.Repeater:

					//local nexus don't propagate through the hubs, as the assumption is outside events come to every repeater in a local system.
					if n.roomNexus {
						continue
					}

					//we send to other hubs and spokes
					for i := range n.hubRegistry {
						n.hubRegistry[i].Channel <- e.EventWrapper
					}
				case base.Messenger:
					//we send to hubs, spokes and dispatchers
					for i := range n.hubRegistry {
						n.hubRegistry[i].Channel <- e.EventWrapper
					}

					//we only send to one repeatera
					if len(n.repeaterRegistry) > 0 {
						curRepeater = (curRepeater + 1) % len(n.repeaterRegistry)
						log.L.Debugf("sending to repeater: %v", curRepeater)
						n.repeaterRegistry[curRepeater].Channel <- e.EventWrapper
					} else {
						log.L.Infof("No repeaters registered")
					}

				default:
					//discard
					continue
				}

				//end case incomingchannel
			case r := <-n.registrationChannel:
				switch r.Type {
				case base.Messenger:
					if r.Create {
						n.registerMessenger(r)
					} else {
						n.deregisterMessenger(r)
					}
				case base.Repeater:
					if r.Create {
						n.repeaterRegistry = addToRegistration(r, n.repeaterRegistry)
					} else {
						n.repeaterRegistry = removeFromRegistration(r, n.repeaterRegistry)
					}
				case base.Hub:
					if r.Create {
						n.hubRegistry = addToRegistration(r, n.hubRegistry)
					} else {
						n.hubRegistry = removeFromRegistration(r, n.hubRegistry)
					}
				default:
					log.L.Errorf("Attempt to register an unknown type: %v", r.Type)
				}
				//end case registrationChannel
			}
		}
	})
}

//not threadsafe
func (n *Nexus) registerMessenger(r base.RegistrationChange) {
	log.L.Infof("Registering messenger %v for rooms %v", r.ID, r.Rooms)
	//add
	for _, cur := range r.Rooms {
		v, ok := n.messengerRegistry[cur]
		if !ok {
			//it doesn't exist
			n.messengerRegistry[cur] = []base.Registration{r.Registration}
			n.roomMessengerIndex[r.ID] = append(n.roomMessengerIndex[r.ID], cur)
			continue
		}
		//it does exist, go through and make sure that the registration(s) don't have duplicate(s)
		for i := range v {
			if v[i].ID == r.ID {
				//duplicate, abort
				log.L.Infof("attempt to create duplicate registration: %v:%v", cur, r.ID)
				continue
			}
		}
		//it doesn't exist just create
		n.messengerRegistry[cur] = append(v, r.Registration)
		n.roomMessengerIndex[r.ID] = append(n.roomMessengerIndex[r.ID], cur)
		continue
	}
	log.L.Infof("Successfully registered messenger %v for rooms %v", r.ID, r.Rooms)
}

//not threadsafe
func (n *Nexus) deregisterMessenger(r base.RegistrationChange) {
	if len(r.Rooms) == 0 {
		//unregister all for this messenger
		r.Rooms = n.roomMessengerIndex[r.ID]
	}

	for _, cur := range r.Rooms {
		log.L.Infof("Unregistering messenger %v for rooms %v", r.ID, r.Rooms)
		v, ok := n.messengerRegistry[cur]
		if !ok {
			//it doesn't exist
			log.L.Infof("Trying to remove unknown registration: %v:%v", cur, r.ID)
			continue
		}
		//it does exist, find it to be removed
		for i := range v {
			if v[i].ID == r.ID {
				log.L.Infof("Removing messenger registration", cur, r.ID)
				//remove it
				v[i] = v[len(v)-1]
				n.messengerRegistry[cur] = v[:len(v)-1]

				//remove from the index
				index := n.roomMessengerIndex[r.ID]
				for j := range index {
					if index[j] == cur {
						index[i] = index[len(index)-1]
						n.roomMessengerIndex[r.ID] = index[:len(index)-1]
					}
				}
			}
		}
		//it doesn't exist
		log.L.Infof("Trying to remove unknown registration: %v:%v", cur, r.ID)
		continue
	}
}

//don't call outside of the start function. Not threadsafe
func addToRegistration(r base.RegistrationChange, registry []base.Registration) []base.Registration {
	log.L.Infof("Registering %v %v", r.Type, r.ID)
	for i := range registry {
		if registry[i].ID == r.ID {
			log.L.Infof("Attempting to register duplicate %v: %v", r.Type, r.ID)
			return registry
		}
	}
	//not there, add it
	return append(registry, r.Registration)
}

//don't call outside of the start function. Not threadsafe
func removeFromRegistration(r base.RegistrationChange, registry []base.Registration) []base.Registration {

	//we're deleting
	log.L.Infof("unregistering %v %v", r.Type, r.ID)
	for i := range registry {
		if registry[i].ID == r.ID {
			log.L.Infof("Removing %v registration %v ", r.Type, r.ID)

			//remove it
			registry[i] = registry[len(registry)-1]
			registry = registry[:len(registry)-1]

			return registry
		}
	}
	log.L.Infof("Attempting to delete non-existent %v: %v", r.Type, r.ID)
	return registry

}

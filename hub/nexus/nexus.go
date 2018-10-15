package nexus

import (
	"sync"

	"github.com/byuoitav/central-event-system/hub/base"
	"github.com/byuoitav/common/log"
	"github.com/byuoitav/common/nerr"
)

//N is the default Nexus
//Do this in server.go?
var N *Nexus

func init() {
	N = &Nexus{
		registrationChannel: make(chan base.RegistrationChange),
		incomingChannel:     make(chan base.HubEventWrapper),
	}
	//start the router
	go N.start()
}

//Nexus handles the actuall routing of events around
type Nexus struct {
	messengerRegistry   map[string][]base.Registration
	hubRegistry         []base.Registration
	broadcasterRegistry []base.Registration

	registrationChannel chan base.RegistrationChange
	incomingChannel     chan base.HubEventWrapper

	once sync.Once
}

//RegisterConnection in cases of spokes is called with a set of rooms, and the channel to send events for that room down. in cases of dispatchers and hubs the rooms array is ignored.
func (n *Nexus) RegisterConnection(rooms []string, channel chan base.EventWrapper, connID, connType string) *nerr.E {
	n.registrationChannel <- base.RegistrationChange{
		Create: true,
		Type:   connType,
		Rooms:  rooms,
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
		Create: false,
		Registration: base.Registration{
			ID: connID,
		},
		Type:  connType,
		Rooms: rooms,
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
	n.once.Do(func() {
		for {
			select {
			case e := <-n.incomingChannel:
				if v, ok := n.messengerRegistry[e.Room]; ok {
					//we ALWAYS send to spokes
					for i := range v {
						if e.Source != base.Messenger || v[i].ID != e.SourceID {
							v[i].Channel <- e.EventWrapper
						}
					}
					//where else do we send it?
					switch e.Source {
					case base.Receiver:
						//we send to other hubs and spokes
						for i := range n.hubRegistry {
							n.hubRegistry[i].Channel <- e.EventWrapper
						}
					case base.Messenger:
						//we send to hubs, spokes and dispatchers
						for i := range n.hubRegistry {
							n.hubRegistry[i].Channel <- e.EventWrapper
						}
						for i := range n.broadcasterRegistry {
							n.broadcasterRegistry[i].Channel <- e.EventWrapper
						}
					default:
						//discard
						continue
					}
				}

				//end case incomingchannel
			case r := <-n.registrationChannel:
				switch r.Type {
				case base.Messenger:
					if r.Create {
						n.registerSpoke(r)
					} else {
						n.deregisterSpoke(r)
					}
				case base.Broadcaster:
					if r.Create {
						n.broadcasterRegistry = addToRegistration(r, n.broadcasterRegistry)
					} else {
						n.broadcasterRegistry = removeFromRegistration(r, n.broadcasterRegistry)
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
func (n *Nexus) registerSpoke(r base.RegistrationChange) {
	log.L.Infof("Registering spoke %v for rooms %v", r.ID, r.Rooms)
	//add
	for _, cur := range r.Rooms {
		v, ok := n.messengerRegistry[cur]
		if !ok {
			//it doesn't exist
			n.messengerRegistry[cur] = []base.Registration{r.Registration}
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
		continue
	}
	log.L.Infof("Successfully registered spoke %v for rooms %v", r.ID, r.Rooms)
}

//not threadsafe
func (n *Nexus) deregisterSpoke(r base.RegistrationChange) {

	for _, cur := range r.Rooms {
		log.L.Infof("Unregistering spoke %v for rooms %v", r.ID, r.Rooms)
		v, ok := n.messengerRegistry[cur]
		if !ok {
			//it doesn't exist
			log.L.Infof("Trying to remove unknown registration: %v:%v", cur, r.ID)
			continue
		}
		//it does exist, find it to be removed
		for i := range v {
			if v[i].ID == r.ID {
				log.L.Infof("Removing spoke registration", cur, r.ID)
				//remove it
				v[i] = v[len(v)-1]
				n.messengerRegistry[cur] = v[:len(v)-1]
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

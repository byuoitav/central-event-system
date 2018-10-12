package axle

import (
	"sync"

	"github.com/byuoitav/central-event-system/hub/base"
	"github.com/byuoitav/common/log"
	"github.com/byuoitav/common/nerr"
)

//A is the default Axle
//Do this in server.go?
var A *Axle

func init() {
	A = Axle{
		registrationChannel: make(chan base.RegistrationChange),
		incomingChannel:     make(chan base.HubEventWrapper),
	}
	//start the router
	go A.start()
}

//Axle handles the actuall routing of events around
type Axle struct {
	spokeRegistry      map[string][]base.Registration
	hubRegistry        []base.Registration
	dispatcherRegistry []base.Registration

	registrationChannel chan base.RegistrationChange
	incomingChannel     chan base.HubEventWrapper

	once sync.Once
}

//RegisterConnection in cases of spokes is called with a set of rooms, and the channel to send events for that room down. in cases of dispatchers and hubs the rooms array is ignored.
func (a *Axle) RegisterConnection(rooms []string, channel chan<- base.EventWrapper, connID, connType string) *nerr.E {
	a.registrationChannel <- base.RegistrationChange{
		Create:  true,
		ID:      connID,
		Type:    connType,
		Rooms:   rooms,
		Channel: channel,
	}
	return nil
}

//DeregisterConnection will deregsiter the provided connection (type + ID) fro all rooms provided. In cases of dispatchers and hubs the rooms parameter is ignored
func (a *Axle) DeregisterConnection(rooms []string, connType, connID string) *nerr.E {

	a.registrationChannel <- base.RegistrationChange{
		Create: false,
		ID:     connID,
		Type:   connType,
		Rooms:  rooms,
	}
	return nil
}

//Submit sends an event to the hub for routing
func (a *Axle) Submit(e EventWrapper, Source, SourceID string) *nerr.E {

	if len(source) == 0 || len(SourceID) == 0 {
		return nerr.Create("Can't submit blank source or sourceID", "invalid")
	}

	a.incomingChannel <- base.HubEventWrapper{
		Source:   Source,
		SourceID: SourceID,
		EventWrapper, e,
	}

	return nil
}

func (a *Axle) start() {
	a.once.Do(func() {
		for {
			select {
			case e := <-a.incomingChannel:
				if v, ok := a.spokeRegistry[e.Room]; ok {
					//we ALWAYS send to spokes
					for i := range v {
						if e.Source != base.Spoke || v[i].ID != e.SourceID {
							v[i].Channel <- e.EventWrapper
						}
					}
					//where else do we send it?
					switch e.Source {
					case base.Ingester:
						//we send to other hubs and spokes
						for i := range a.hubRegistry {
							a.hubRegistry[i].Channel <- e
						}
					case base.Spoke:
						//we send to hubs, spokes and dispatchers
						for i := range hubRegistry {
							a.hubRegistry[i].Channel <- e
						}
						for i := range dispatcherRegistry {
							a.dispatcherRegistry[i].Channel <- e
						}
					default:
						//discard
						continue
					}
				}

				//end case incomingchannel
			case r := <-a.registrationChannel:
				switch r.Type {
				case base.Spoke:
					if r.Create {
						a.registerSpoke(r)
					} else {
						a.deregisterSpoke(r)
					}
				case base.Dispatcher:
					if r.Create {
						a.dispatcherRegistry = addToRegistration(r, a.dispatcherRegistry)
					} else {
						a.dispatcherRegistry = removeFromRegistration(r, a.dispatcherRegistry)
					}
				case base.Hub:
					if r.Create {
						a.hubRegistry = addToRegistration(r, a.hubRegistry)
					} else {
						a.hubRegistry = removeFromRegistration(r, a.hubRegistry)
					}
				default:
					log.L.Errorf("Attempt to register an unknown type: %v", r.Type)
				}
				//end case registrationChannel
			}
		}
	}())
}

//not threadsafe
func (a *Axle) registerSpoke(base.RegistrationChange) {
	log.L.Infof("Registering spoke %v for rooms %v", r.ID, r.Rooms)
	//add
	for _, cur := range r.Rooms {
		v, ok := spokeRegistry[cur]
		if !ok {
			//it doesn't exist
			spokeRegistry[cur] = []base.Registration{r.Registration}
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
		spokeRegistry[cur] = append(v, r.Registration)
		continue
	}
	log.L.Infof("Successfully registered spoke %v for rooms %v", r.ID, r.Rooms)
	continue
}

//not threadsafe
func (a *Axle) deregisterSpoke(base.RegistrationChange) {

	for _, cur := range r.Rooms {
		log.L.Infof("Unregistering spoke %v for rooms %v", r.ID, r.Rooms)
		v, ok := spokeRegistry[cur]
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
				v[i] = v[len(a)-1]
				spokeRegistry[cur] = v[:len(a)-1]
			}
		}
		//it doesn't exist
		log.L.Infof("Trying to remove unknown registration: %v:%v", cur, r.ID)
		continue
	}
}

//don't call outside of the start function. Not threadsafe
func addToRegistration(r base.RegistrationChange, registry []base.Registration) []base.Registration {
	var present bool
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
	var deleted bool
	for i := range registry {
		if registry[i].ID == r.ID {
			log.L.Infof("Removing %v registration %v ", r.Type, r.ID)
			//remove it
			v[i] = v[len(a)-1]
			registry[cur] = v[:len(a)-1]

			return registry
		}
	}
	log.L.Infof("Attempting to delete non-existent %v: %v", r.Type, r.ID)
	return registry

}

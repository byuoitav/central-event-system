package router

import (
	"sync"

	"github.com/byuoitav/central-event-system/hub/base"
	"github.com/byuoitav/common/log"
)

func init() {
	registrationChannel = make(chan base.RegistrationChange)
	incomingChannel = make(chan base.HubEventWrapper)
	//start the router
	go start()
}

var (
	spokeRegistry      map[string][]base.Registration
	hubRegistry        []base.Registration
	dispatcherRegistry []base.Registration

	registrationChannel chan base.RegistrationChange
	incomingChannel     chan base.HubEventWrapper
)

//RegisterSpoke is called with a set of rooms, and the channel to send events for that room down.
func RegisterSpoke(rooms []string, channel chan<- base.EventWrapper) error {
	registrationChannel <- base.RegistrationChange{
		Type:    base.Spoke,
		Rooms:   rooms,
		Channel: channel,
	}
	return nil
}

var once sync.Once

func start() {
	once.Do(func() {
		for {
			select {
			case e := <-incomingChannel:
				//got to distrbute the channel

			case r := <-registrationChannel:
				switch r.Type {
				case base.Spoke:
					if r.Create {
						registerSpoke(r)
					} else {
						deregisterSpoke(r)
					}
				case base.Dispatcher:
					if r.Create {
						dispatcherRegistry = addToRegistration(r, dispatcherRegistry)
					} else {
						dispatcherRegistry = removeFromRegistration(r, dispatcherRegistry)
					}
				case base.Hub:
					if r.Create {
						hubRegistry = addToRegistration(r, hubRegistry)
					} else {
						hubRegistry = removeFromRegistration(r, hubRegistry)
					}
				default:
					log.L.Errorf("Attempt to register an unknown type: %v", r.Type)
				}

			}

		}
	}())
}

//don't call outside of the start function. Not threadsafe
func registerSpoke(base.RegistrationChange) {
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

//don't call outside of the start function. Not threadsafe
func deregisterSpoke(base.RegistrationChange) {

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

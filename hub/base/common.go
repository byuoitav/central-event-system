package base

import "github.com/byuoitav/central-event-system/hub/base"

//Constants for the sources/types
const (
	//Spoke .
	Spoke      = "spoke"
	Dispatcher = "dispatcher"
	Ingester   = "ingester"
	Hub        = "hub"
)

//EventWrapper is the wrapper class to handle an event and its tag to avoid unmarshaling overheads.
type EventWrapper struct {
	Room  string
	Event []byte
}

//HubEventWrapper is just an event wrapper plus a source to help with routing within the axle.
type HubEventWrapper struct {
	EventWrapper
	Source   string
	SourceID string
}

//RegistrationChange is used to register or deregister for events
//during deregistration the axle will close the channel, letting the client know that it's safe to exit.
type RegistrationChange struct {
	Registration
	Type   string
	Rooms  []string
	Create bool //if False means to deregister, false means add the registration
}

//Registration contains information needed to maintain a registration. Both ID and Channel are necessary when submitting a regristation change for a new registration. Only ID is necessary during a deregistration request.
type Registration struct {
	ID      string //ID is used to identify a specific channel during de-registration events
	Channel chan base.EventWrapper
}

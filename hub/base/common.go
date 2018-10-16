package base

import (
	"bytes"
	"fmt"

	"github.com/byuoitav/common/log"
	"github.com/byuoitav/common/nerr"
)

//Constants for the sources/types
const (
	//Spoke .
	Messenger   = "messenger"
	Broadcaster = "broadcaster"
	Receiver    = "reciever"
	Hub         = "hub"
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
	Create bool //if False means to deregister, true means add the registration
}

//Registration contains information needed to maintain a registration. Both ID and Channel are necessary when submitting a regristation change for a new registration. Only ID is necessary during a deregistration request.
type Registration struct {
	ID      string //ID is used to identify a specific channel during de-registration events
	Channel chan EventWrapper
}

/*
ParseMessage will take a byte array in format of:
roomID\n
JSONEvent
and parse it out
*/
func ParseMessage(b []byte) (EventWrapper, *nerr.E) {

	//parse out room name
	index := bytes.IndexByte(b, '\n')
	if index == -1 {
		log.L.Errorf("Invalid message format: %v", b)
		return EventWrapper{}, nerr.Create(fmt.Sprintf("Invalid format %s", b), "invalid-format")
	}

	return EventWrapper{
		Room:  string(b[:index]),
		Event: b[index:],
	}, nil
}

//PrepareMessage will take an eventWrapper and return it in the format listed above
func PrepareMessage(message EventWrapper) []byte {
	return append([]byte(message.Room+"\n"), message.Event...)
}

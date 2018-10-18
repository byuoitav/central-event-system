package base

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/byuoitav/common/log"
	"github.com/byuoitav/common/nerr"
	"github.com/byuoitav/common/v2/events"
)

//Constants for the sources/types
const (
	//Spoke .
	Messenger = "messenger"
	Repeater  = "repeater"
	Hub       = "hub"
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
	SubscriptionChange
	Type string `json:"type"`
}

//SubscriptionChange is used to transmit room subscription changes from the messengers to the hub
type SubscriptionChange struct {
	Rooms  []string `json:"rooms"`
	Create bool     `json:"create"` //if False means to deregister, true means add the registration
}

//Registration contains information needed to maintain a registration. Both ID and Channel are necessary when submitting a regristation change for a new registration. Only ID is necessary during a deregistration request.
type Registration struct {
	ID      string            `json:"id,omitempty"` //ID is used to identify a specific channel during de-registration events
	Channel chan EventWrapper `json:"-"`
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

//WrapEvent takes an event and wraps it in the wrappe for use in the central event system
func WrapEvent(e events.Event) EventWrapper {

	b, err := json.Marshal(e)
	if err != nil {
		log.L.Errorf("Couldn't marshal event %v", err.Error())
		return EventWrapper{}
	}
	return EventWrapper{
		Room:  e.AffectedRoom.RoomID,
		Event: b,
	}
}

//UnwrapEvent .
func UnwrapEvent(e EventWrapper) (events.Event, *nerr.E) {
	var ev events.Event
	err := json.Unmarshal(e.Event, &ev)
	if err != nil {
		return ev, nerr.Translate(err).Addf("Couldn't unwrap event")
	}
	return ev, nil
}

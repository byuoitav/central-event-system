package repeater

import (
	"github.com/byuoitav/central-event-system/hub/base"
	"github.com/byuoitav/central-event-system/messenger"
	"github.com/byuoitav/common/nerr"
	"github.com/byuoitav/common/v2/events"
)

//Repeater .
type Repeater struct {
	messenger   messenger.Messenger
	connections map[string]ConnectionManager
}

var mess messenger.Messenger

//SendEvent will take an event and format it in the proper way to translate to the hub format
func SendEvent(e events.Event) *nerr.E {
	mess.SendEvent(base.WrapEvent(e))
	return nil
}

//ReadEvent .
func ReadEvent() (events.Event, *nerr.E) {
	return base.UnwrapEvent(<-mess.ReceiveEvent())
}

# Central Event System

In the central event system there are three major players

1. Hub 
2. Repeaters
3. Messengers

The roles are discussed below.

### Hub

In the central event system the hub acts as the router, or distributor for events, taking events from repeaters and messengers, and redistriubuting them. The hub connects to messengers and repeaters via websockets, and each message is in the format of 

```
ROOMID\n
JSONEvent\n
```

Where the RoomID acts as the routing tag which controls which messengers the event is sent to, as well as where repeaters will send the event. 

All events that flow into a hub are sent to at most one repeater (each event will be sent to a single repeater, but there may be multiple repeaters), and the repeaters determine if the event is to be routed to outside devices. 

When a messenger is spun up, it will establish a connection with the hub. Similar to event nodes, a messenger is not a purpose-built server, but other services become messengers via the messenger package. Messengers 'register' rooms for which they would like to recieve events, the Hub maintains this list and will only send events to messengers who have registered to recieve events for that room. 

Repeaters and messengers should be matched to at most one hub, but there may be multiple hubs

There are three sources for a hub. Routing based on source is as follows:


|Source|Destination(s)|
|------|--------------|
|Hub|Messengers|
|Messengers|Hubs, Messengers, Repeaters|
|Repeaters|Messengers, Hubs|

### Repeaters

Repeaters provide a websocket endpoint to insert events into the central event system. Outside services connect to the repeaters, and send v2 events up the webscoket. The websocket is persisted for a set period of time after an event is sent, after the period of time elapses without being used, the socket is closed. 

The repeater then adds the tag to the event, and forwards it to a hub. In addition repeaters accept events from the hub and forward it to outside systems - the current use case for these are to send events that originate in the cloud to systems in room. 

Connections to repeaters can be round robined. An repeater is  a special case of a messenger in that it does not send subscriptions to the hub, and the hub will ensure that events are sent to at least one repeater.

### Messenger

A messenger is a package that enables systems to have two way connection with the central event system. It is intended that systems will establish a websocket with a messenger, and those systems will correspond to a subset of rooms, and they will subscribe to the events. 

There may be 'write-only' messengers who subscribe to no events. The primary difference between a write-only messenger and an ingester is that events that flow through an ingester are handled as originating outside of the central event system, and thus will not be forwarded to the dispatchers. In addition the websockets from a messenger may or may not be persistent. 

### Hub interconnection. 

Hubs must be manually interconnected on startup. You can do this by sending a request to /interconnect/:address with the address of the second router to connect to. You should only send one request per interconnection. 

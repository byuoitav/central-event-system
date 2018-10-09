# Central Event System

In the central event system there are four major players

1. Hub 
2. Ingesters
3. Dispatchers 
4. Spokes

The roles are discussed below

### Hub

In the central event system the hub acts as the router, or distributor for events, taking events from ingesters and spokes, and sending them out to dispatchers and spokes. The hub connects to spokes, ingesters, and distributors via websockets, and each message is in the format of 

```
ROOMID\n
JSONEvent\n
```

Where the RoomID acts as the routing tag which controls which spokes the event is sent to, as well as where dispatchers will send the event. 

All events that flow into a hub are sent to a at most one dispatcher (each event will be sent to a single dispatcher, but there may be multiple dispatchers), and the dispatchers determine if the event is to be routed to outside devices. 

When a spoke is spun up it will establish a connection with the hub. Similar to event nodes a spoke is not a purpose built server, but other services become spokes via the spoke package. Spokes 'register' rooms for which they would like to recieve events, the Hub maintains this list and will only send events to spokes who have registered to recieve events for that room. 

Ingesters should be matched to at most one hub, but there may be multiple hubs - spokes must connect to all hubs to ensure event delivery. 

### Ingesters

Ingesters provide a websocket endpoint to insert events into the central event system. Outside services connect to the ingesters, and send v2 events up the webscoket. The websocket is persisted for a set period of time after an event is sent, after the period of time elapses without being used, the socket is closed. 

The ingester then adds the tag to the event, and forwards it to a hub. Connections to ingesters can be round robined. 

### Dispatchers

Dispatchers accept events from the hub and forward it to outside systems - the current use case for these are to send events that originate in the cloud to systems in room. 

### Spoke

A spoke is a package that enables systems to have two way connection with the central event system. It is intended that systems will establish a websocket with a spoke, and those systems will correspond to a subset of rooms, and they will subscribe to the events. 

There may be 'write-only' spokes who subscribe to no events. The primary difference between a write-only spoke and an ingester is that events that flow through an ingester are handled as originating outside of the central event system, and thus will not be forwarded to the dispatchers. In addition the websockets from a spoke may or may not be persistent. 


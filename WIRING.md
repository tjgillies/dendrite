# Wiring

The diagram is incomplete. The following things aren't shown on the diagram:

 - [ ] Device Messages
 - [ ] User Profiles
 - [ ] Notification Counts
 - [ ] Sending federation.
 - [ ] Querying federation.
 - [ ] Other things that aren't shown on the diagram.

Diagram:


    W -> Writer
    S -> Server/Store/Service/Something/Stuff
    R -> Reader

               +---+                                                    +---+                              +---+
    +----------| W |                                         +----------| S |                     +--------| R |
    |          +---+                                         | Receipts +---+                     | Client +---+
    | Federation |>=========================================>| Server     |>=====================>| Sync     |
    | Send       |                                           |            |                       |          |
    |            |                                 +---+     |            |                       |          |
    |            |                        +--------| W |     |            |                       |          |
    |            |                        | Client +---+     |            |                       |          |
    |            |                        | Receipt  |>=====>|            |                       |          |
    |            |                        | Updater  |       |            |                       |          |
    |            |                        +----------+       |            |                       |          |
    |            |                                           |            |                       |          |
    |            |                +---+            +---+     |            |                +---+  |          |
    |            |   +------------| W |     +------| S |     |            |       +--------| R |  |          |
    |            |   | Federation +---+     | Room +---+     |            |       | Client +---+  |          |
    |            |   | Backfill     |>=====>| Server |>=====>|            |>=====>| Push     |    |          |
    |            |   +--------------+       |        |       +------------+       |          |    |          |
    |            |                          |        |                            |          |    |          |
    |            |                          |        |>==========================>|          |    |          |
    |            |                          |        |                            +----------+    |          |
    |            |                          |        |                                            |          |
    |            |                          |        |                                     +---+  |          |
    |            |                          |        |                            +--------| R |  |          |
    |            |                          |        |                            | Client +---+  |          |
    |            |>========================>|        |>==========================>| Search   |    |          |
    |            |                          |        |                            |          |    |          |
    |            |                          |        |                            +----------+    |          |
    |            |                          |        |                                            |          |
    |            |                          |        |>==========================================>|          |
    |            |                          |        |                                            |          |
    |            |                +---+     |        |                  +---+                     |          |
    |            |       +--------| W |     |        |       +----------| S |                     |          |
    |            |       | Client +---+     |        |       | Presence +---+                     |          |
    |            |       | Room     |>=====>|        |>=====>| Server     |>=====================>|          |
    |            |       | Send     |       +--------+       |            |                       |          |
    |            |       |          |                        |            |                       |          |
    |            |       |          |>======================>|            |<=====================<|          |
    |            |       +----------+                        |            |                       |          |
    |            |                                           |            |                       |          |
    |            |                                 +---+     |            |                       |          |
    |            |                        +--------| W |     |            |                       |          |
    |            |                        | Client +---+     |            |                       |          |
    |            |                        | Presence |>=====>|            |                       |          |
    |            |                        | Setter   |       |            |                       |          |
    |            |                        +----------+       |            |                       |          |
    |            |                                           |            |                       |          |
    |            |                                           |            |                       |          |
    |            |>=========================================>|            |                       |          |
    |            |                                           +------------+                       |          |
    |            |                                                                                |          |
    |            |                                                      +---+                     |          |
    |            |                                           +----------| S |                     |          |
    |            |                                           | Typing   +---+                     |          |
    |            |>=========================================>| Server     |>=====================>|          |
    +------------+                                           |            |                       +----------+
                                                   +---+     |            |
                                          +--------| W |     |            |
                                          | Client +---+     |            |
                                          | Typing   |>=====>|            |
                                          | Setter   |       |            |
                                          +----------+       +------------+


# Component Descriptions

Many of the components are logical rather than physical. For example it is
possible that all of the client API writers will end up being glued together
and always deployed as a single unit.

Outbound federation requests will probably need to be funnelled through a
choke-point to implement ratelimiting and backoff correctly.

## Federation Send

 * Handles `/federation/v1/send/` requests.
 * Fetches missing ``prev_events`` from the remote server if needed.
 * Fetches missing room state from the remote server if needed.
 * Checks signatures on remote events, downloading keys if needed.
 * Queries information needed to process events from the Room Server.
 * Writes room events to logs.
 * Writes presence updates to logs.
 * Writes receipt updates to logs.
 * Writes typing updates to logs.
 * Writes other updates to logs.

## Client Room Send

 * Handles puts to `/client/v1/rooms/` that create room events.
 * Queries information needed to process events from the Room Server.
 * Talks to remote servers if needed for joins and invites.
 * Writes room event pdus.
 * Writes presence updates to logs.

## Client Presence Setter

 * Handles puts to whatever the client API path for presence is?
 * Writes presence updates to logs.

## Client Typing Setter

 * Handles puts to whatever the client API path for typing is?
 * Writes typing updates to logs.

## Client Receipt Updater

 * Handles puts to whatever the client API path for receipts is?
 * Writes typing updates to logs.

## Federation Backfill

 * Backfills events from other servers
 * Writes the resulting room events to logs.
 * Is a different component from the room server itself cause it'll
   be easier if the room server component isn't making outbound HTTP requests
   to remote servers

## Room Server

 * Reads new and backfilled room events from the logs written by FS, FB and CRS.
 * Tracks the current state of the room and the state at each event.
 * Probably does auth checks on the incoming events.
 * Handles state resolution as part of working out the current state and the
 * state at each event.
 * Writes updates to the current state and new events to logs.
 * Shards by room ID.

## Receipt Server

 * Reads new updates to receipts from the logs written by the FS and CRU.
 * Somehow learns enough information from the room server to workout how the
   current receipt markers move with each update.
 * Writes the new marker positions to logs
 * Shards by room ID?
 * It may be impossible to implement without folding it into the Room Server
   forever coupling the components together.

## Typing Server

 * Reads new updates to typing from the logs written by the FS and CTS.
 * Updates the current list of people typing in a room.
 * Writes the current list of people typing in a room to the logs.
 * Shards by room ID?

## Presence Server

 * Reads the current state of the rooms from the logs to track the intersection
   of room membership between users.
 * Reads updates to presence from the logs writen by the FS and the CPS.
 * Reads when clients sync from the logs from the Client Sync.
 * Tracks any timers for users.
 * Writes the changes to presence state to the logs.
 * Shards by user ID somehow?

## Client Sync

 * Handle /client/v2/sync requests.
 * Reads new events and the current state of the rooms from logs written by the Room Server.
 * Reads new receipts positions from the logs written by the Receipts Server.
 * Reads changes to presence from the logs written by the Presence Server.
 * Reads changes to typing from the logs written by the Typing Server.
 * Writes when a client starts and stops syncing to the logs.

## Client Search

 * Handle whatever the client API path for event search is?
 * Reads new events and the current state of the rooms from logs writeen by the Room Server.
 * Maintains a full text search index of somekind.

## Client Push

 * Pushes unread messages to remote push servers.
 * Reads new events and the current state of the rooms from logs writeen by the Room Server.
 * Reads the position of the read marker from the Receipts Server.
 * Makes outbound HTTP hits to the push server for the client device.

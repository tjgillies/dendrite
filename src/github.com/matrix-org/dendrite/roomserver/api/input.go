package api

const (
	// Outlier events fall outside the contiguous event graph.
	// We do not have the state for these events.
	// These events are state events used to authenticate other events.
	// They can become part of the contiguous event graph via backfill.
	KindOutlier = 1
	// Join events start a new contiguous event graph. The first event
	// in the list must be a m.room.memeber event joining this server to
	// the room. There must be a copy of the
	KindJoin = 2
	// New events extend the contiguous graph going forwards.
	// They usually don't need state, but may include state if the
	// there was a new event that references an event that we don't
	// have a copy of.
	KindNew = 3
	// Backfilled events extend the contiguous graph going backwards.
	// They always have state.
	KindBackfill = 4
)

type InputEvent struct {
	// Whether these events are new, backfilled or outliers.
	Kind int
	// The event JSON for each event to add.
	Event EventJSON
	// Optional list of state events forming the state before this event.
	// These state events must have already been persisted.
	State []string
}

type InputPurgeHistory struct {
	// The room_id to remove history from.
	RoomID string
	// The depth to purge history up to.
	Depth int64
}

type InputRedact struct {
	// List of events to redact.
	EventIDs []string
}

package storage

import (
	"database/sql"
	"fmt"
	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/roomserver/types"
)

const stateSnapshotSchema = `
-- The state of a room before an event.
-- Stored as a list of state_block entries stored in a separate table.
-- The actual state is constructed by combining all the state_block entries
-- referenced by state_block_nids together. If the same state key tuple appears
-- multiple times then the entry from the later state_block clobbers the earlier
-- entries.
-- This encoding format allows us to implement a delta encoding which is useful
-- because room state tends to accumulate small changes over time. Although if
-- the list of deltas becomes too long it becomes more efficient to encode
-- the full state under single state_block_nid.
CREATE SEQUENCE IF NOT EXISTS state_snapshot_nid_seq;
CREATE TABLE IF NOT EXISTS state_snapshots (
    -- Local numeric ID for the state.
    state_snapshot_nid bigint PRIMARY KEY DEFAULT nextval('state_snapshot_nid_seq'),
    -- Local numeric ID of the room this state is for.
    -- Unused in normal operation, but useful for background work or ad-hoc debugging.
    room_nid bigint NOT NULL,
    -- List of state_block_nids, stored sorted by state_block_nid.
    state_block_nids bigint[] NOT NULL
);
`

const insertStateSQL = "" +
	"INSERT INTO state_snapshots (room_nid, state_block_nids)" +
	" VALUES ($1, $2)" +
	" RETURNING state_snapshot_nid"

// Bulk state data NID lookup.
// Sorting by state_snapshot_nid means we can use binary search over the result
// to lookup the state data NIDs for a state snapshot NID.
const bulkSelectStateBlockNIDsSQL = "" +
	"SELECT state_snapshot_nid, state_block_nids FROM state_snapshots" +
	" WHERE state_snapshot_nid = ANY($1) ORDER BY state_snapshot_nid ASC"

type stateSnapshotStatements struct {
	insertStateStmt              *sql.Stmt
	bulkSelectStateBlockNIDsStmt *sql.Stmt
}

func (s *stateSnapshotStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(stateSnapshotSchema)
	if err != nil {
		return
	}
	if s.insertStateStmt, err = db.Prepare(insertStateSQL); err != nil {
		return
	}
	if s.bulkSelectStateBlockNIDsStmt, err = db.Prepare(bulkSelectStateBlockNIDsSQL); err != nil {
		return
	}
	return
}

func (s *stateSnapshotStatements) insertState(roomNID types.RoomNID, stateBlockNIDs []types.StateBlockNID) (stateNID types.StateSnapshotNID, err error) {
	nids := make([]int64, len(stateBlockNIDs))
	for i := range stateBlockNIDs {
		nids[i] = int64(stateBlockNIDs[i])
	}
	err = s.insertStateStmt.QueryRow(int64(roomNID), pq.Int64Array(nids)).Scan(&stateNID)
	return
}

func (s *stateSnapshotStatements) bulkSelectStateBlockNIDs(stateNIDs []types.StateSnapshotNID) ([]types.StateBlockNIDList, error) {
	nids := make([]int64, len(stateNIDs))
	for i := range stateNIDs {
		nids[i] = int64(stateNIDs[i])
	}
	rows, err := s.bulkSelectStateBlockNIDsStmt.Query(pq.Int64Array(nids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make([]types.StateBlockNIDList, len(stateNIDs))
	i := 0
	for ; rows.Next(); i++ {
		result := &results[i]
		var stateBlockNIDs pq.Int64Array
		if err := rows.Scan(&result.StateSnapshotNID, &stateBlockNIDs); err != nil {
			return nil, err
		}
		result.StateBlockNIDs = make([]types.StateBlockNID, len(stateBlockNIDs))
		for k := range stateBlockNIDs {
			result.StateBlockNIDs[k] = types.StateBlockNID(stateBlockNIDs[k])
		}
	}
	if i != len(stateNIDs) {
		return nil, fmt.Errorf("storage: state NIDs missing from the database (%d != %d)", i, len(stateNIDs))
	}
	return results, nil
}

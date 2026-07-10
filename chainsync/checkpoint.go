package chainsync

// Checkpoint is the two values needed to detect a reorg: not just how far
// we've processed, but the hash of the block we last trusted at that position,
// so a future block's ParentHash can be checked against it.
//
// Persistence lives in storage.LoadCheckpoint/RecordBlock/Rollback, not here -
// the checkpoint has to be updated in the same SQLite transaction as the data
// it's tracking progress against, which a standalone file could never do.
type Checkpoint struct {
	BlockNumber uint64
	BlockHash   string
}

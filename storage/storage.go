package storage

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

// Open connects to (creating if necessary) a SQLite database file at path,
// using modernc.org/sqlite - a pure-Go driver, so this never needs a C
// compiler to build, matching the single-static-binary goal for this project.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return db, nil
}

// InitSchema creates the tables if they don't already exist.
//
// transfers: UNIQUE(tx_hash, log_index) is the same idempotency key
// established earlier in the plan - the pair that actually identifies one
// transfer event uniquely, since a single tx can contain many transfer logs.
//
// blocks: one hash per height, so a reorg can be traced backwards to the
// fork point. Only heights above the finalized block need to be kept -
// finalized blocks can't reorg - so this table gets pruned, not grown
// forever.
//
// checkpoint: a single row (CHECK(id = 1) enforces that at the schema level)
// holding how far we've processed. It has to live here, in the same database
// as blocks/transfers, rather than in a separate file - a separate file can
// never be updated in the same transaction as the data it's tracking progress
// against, which is exactly the crash window that matters most (a reorg
// rollback that deletes rows but doesn't also move the checkpoint back).
// It also can't just be derived from MAX(block_number) in blocks, since
// PruneBlocksBelow empties that table out once everything is finalized.
func InitSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS transfers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			block_number INTEGER NOT NULL,
			log_index INTEGER NOT NULL,
			tx_hash TEXT NOT NULL,
			token_symbol TEXT NOT NULL,
			from_address TEXT NOT NULL,
			to_address TEXT NOT NULL,
			raw_amount TEXT NOT NULL,
			is_final INTEGER NOT NULL,
			UNIQUE(tx_hash, log_index)
		);

		CREATE INDEX IF NOT EXISTS idx_transfers_block ON transfers(block_number);

		CREATE TABLE IF NOT EXISTS blocks (
			block_number INTEGER PRIMARY KEY,
			block_hash TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS checkpoint (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			block_number INTEGER NOT NULL,
			block_hash TEXT NOT NULL
		);
	`)
	return err
}

// Transfer is one row of the transfers table.
type Transfer struct {
	BlockNumber uint64
	LogIndex    uint
	TxHash      string
	TokenSymbol string
	From        string
	To          string
	RawAmount   string
	IsFinal     bool
}

// InsertTransfer records one transfer log. INSERT OR REPLACE because
// re-processing a block is normal - a crash mid-block, or re-indexing after
// a reorg - and hitting the UNIQUE(tx_hash, log_index) constraint on the
// second pass would be a spurious error, not a real one.
func InsertTransfer(db *sql.DB, t Transfer) error {
	_, err := db.Exec(`
		INSERT OR REPLACE INTO transfers (block_number, log_index, tx_hash, token_symbol, from_address, to_address, raw_amount, is_final)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, t.BlockNumber, t.LogIndex, t.TxHash, t.TokenSymbol, t.From, t.To, t.RawAmount, t.IsFinal)
	return err
}

// InsertBlock records the hash seen at a given height. INSERT OR REPLACE
// because re-indexing after a reorg writes a new hash at a height that
// already has a (now stale) row.
func InsertBlock(db *sql.DB, blockNumber uint64, blockHash string) error {
	_, err := db.Exec(`
		INSERT OR REPLACE INTO blocks (block_number, block_hash) VALUES (?, ?)
	`, blockNumber, blockHash)
	return err
}

// RecordBlock atomically records a processed block's hash and advances the
// checkpoint to it, in one transaction. Doing these as two separate calls
// would leave a window where a crash records the block but not the
// checkpoint (or vice versa) - the two would drift out of sync exactly when
// it matters most, on an unclean shutdown.
func RecordBlock(db *sql.DB, blockNumber uint64, blockHash string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`INSERT OR REPLACE INTO blocks (block_number, block_hash) VALUES (?, ?)`, blockNumber, blockHash); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT OR REPLACE INTO checkpoint (id, block_number, block_hash) VALUES (1, ?, ?)`, blockNumber, blockHash); err != nil {
		return err
	}
	return tx.Commit()
}

// LoadCheckpoint returns how far processing has gotten. found is false only
// on a fresh database that has never recorded a block yet - the caller
// should seed a starting point (typically the current chain tip) itself.
func LoadCheckpoint(db *sql.DB) (blockNumber uint64, blockHash string, found bool, err error) {
	err = db.QueryRow(`SELECT block_number, block_hash FROM checkpoint WHERE id = 1`).Scan(&blockNumber, &blockHash)
	if err == sql.ErrNoRows {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, err
	}
	return blockNumber, blockHash, true, nil
}

// GetBlockHash returns the hash recorded at a height. The bool reports
// whether a row existed - a missing height is a normal case (pruned below
// finality, or never seen), not an error.
func GetBlockHash(db *sql.DB, blockNumber uint64) (string, bool, error) {
	var hash string
	err := db.QueryRow(`SELECT block_hash FROM blocks WHERE block_number = ?`, blockNumber).Scan(&hash)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return hash, true, nil
}

// Rollback discards stale transfers and block hashes left behind by an
// orphaned branch, and resets the checkpoint to the fork point, all in one
// transaction. forkPoint is the last height both chains agree on (from
// chainsync.FindForkPoint) - everything strictly above it is discarded.
//
// The checkpoint reset has to be part of the same transaction as the
// deletes, not a follow-up call - a crash between "deleted the stale rows"
// and "moved the checkpoint back" would leave the checkpoint pointing past
// data that no longer exists, which is worse than not rolling back at all.
func Rollback(db *sql.DB, forkPoint uint64, forkPointHash string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM transfers WHERE block_number > ?`, forkPoint); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM blocks WHERE block_number > ?`, forkPoint); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT OR REPLACE INTO checkpoint (id, block_number, block_hash) VALUES (1, ?, ?)`, forkPoint, forkPointHash); err != nil {
		return err
	}
	return tx.Commit()
}

// PruneBlocksBelow drops block hashes at or below the finalized height.
// Finalized blocks can't reorg, so their hashes will never be compared
// against again - keeping them would grow this table forever.
func PruneBlocksBelow(db *sql.DB, finalizedBlock uint64) error {
	_, err := db.Exec(`DELETE FROM blocks WHERE block_number <= ?`, finalizedBlock)
	return err
}

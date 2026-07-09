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

// DeleteAbove removes stale transfers and block hashes left behind by an
// orphaned branch. blockNumber is the fork point - the last height both
// chains agree on - so everything strictly above it is discarded.
//
// Both deletes run in one transaction: a crash between them would otherwise
// leave transfers from a dead branch sitting in the table with no block
// hashes to catch them next time.
func DeleteAbove(db *sql.DB, blockNumber uint64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM transfers WHERE block_number > ?`, blockNumber); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM blocks WHERE block_number > ?`, blockNumber); err != nil {
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

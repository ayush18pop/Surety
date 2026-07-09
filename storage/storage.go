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

// InitSchema creates the transfers table if it doesn't already exist.
// UNIQUE(tx_hash, log_index) is the same idempotency key established
// earlier in the plan - the pair that actually identifies one transfer
// event uniquely, since a single tx can contain many transfer logs.
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

func InsertTransfer(db *sql.DB, t Transfer) error {
	_, err := db.Exec(`
		INSERT INTO transfers (block_number, log_index, tx_hash, token_symbol, from_address, to_address, raw_amount, is_final)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, t.BlockNumber, t.LogIndex, t.TxHash, t.TokenSymbol, t.From, t.To, t.RawAmount, t.IsFinal)
	return err
}

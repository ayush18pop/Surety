package storage

import (
	"path/filepath"
	"testing"
)

func TestInsertAndReadBack(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	want := Transfer{
		BlockNumber: 25483436,
		LogIndex:    3,
		TxHash:      "0xdeadbeef",
		TokenSymbol: "USDC",
		From:        "0xfrom",
		To:          "0xto",
		RawAmount:   "159473363",
		IsFinal:     true,
	}

	if err := InsertTransfer(db, want); err != nil {
		t.Fatalf("InsertTransfer failed: %v", err)
	}

	var got Transfer
	row := db.QueryRow(`
		SELECT block_number, log_index, tx_hash, token_symbol, from_address, to_address, raw_amount, is_final
		FROM transfers WHERE tx_hash = ? AND log_index = ?
	`, want.TxHash, want.LogIndex)

	if err := row.Scan(&got.BlockNumber, &got.LogIndex, &got.TxHash, &got.TokenSymbol, &got.From, &got.To, &got.RawAmount, &got.IsFinal); err != nil {
		t.Fatalf("reading back the inserted row failed: %v", err)
	}

	if got != want {
		t.Fatalf("read back a different row than what was inserted:\n got:  %+v\n want: %+v", got, want)
	}
}

package storage

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// newTestDB opens a fresh schema-initialized database in a temp dir.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}
	return db
}

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

// Re-processing a block is normal (crash mid-block, or re-indexing after a
// reorg), so inserting the same (tx_hash, log_index) twice must not error.
func TestInsertTransferIsIdempotent(t *testing.T) {
	db := newTestDB(t)

	tr := Transfer{
		BlockNumber: 100,
		LogIndex:    0,
		TxHash:      "0xtx",
		TokenSymbol: "USDC",
		From:        "0xfrom",
		To:          "0xto",
		RawAmount:   "1",
	}

	if err := InsertTransfer(db, tr); err != nil {
		t.Fatalf("first InsertTransfer failed: %v", err)
	}
	if err := InsertTransfer(db, tr); err != nil {
		t.Fatalf("re-inserting the same transfer failed: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM transfers`).Scan(&count); err != nil {
		t.Fatalf("counting transfers failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("got %d rows, want 1 - the second insert should replace, not duplicate", count)
	}
}

func TestBlockHashRoundTrip(t *testing.T) {
	db := newTestDB(t)

	if err := InsertBlock(db, 100, "0xaaa"); err != nil {
		t.Fatalf("InsertBlock failed: %v", err)
	}

	hash, ok, err := GetBlockHash(db, 100)
	if err != nil {
		t.Fatalf("GetBlockHash failed: %v", err)
	}
	if !ok || hash != "0xaaa" {
		t.Fatalf("got (%q, %v), want (%q, true)", hash, ok, "0xaaa")
	}

	// A height never seen is a normal miss, not an error.
	if _, ok, err := GetBlockHash(db, 999); err != nil || ok {
		t.Fatalf("missing height: got (ok=%v, err=%v), want (false, nil)", ok, err)
	}
}

// A reorg re-indexes heights that already have rows, so writing the same
// height twice must overwrite rather than fail on the primary key.
func TestInsertBlockOverwritesSameHeight(t *testing.T) {
	db := newTestDB(t)

	if err := InsertBlock(db, 100, "0xold"); err != nil {
		t.Fatalf("first InsertBlock failed: %v", err)
	}
	if err := InsertBlock(db, 100, "0xnew"); err != nil {
		t.Fatalf("overwriting InsertBlock failed: %v", err)
	}

	hash, _, err := GetBlockHash(db, 100)
	if err != nil {
		t.Fatalf("GetBlockHash failed: %v", err)
	}
	if hash != "0xnew" {
		t.Fatalf("got %q, want the overwritten hash %q", hash, "0xnew")
	}
}

func TestDeleteAbove(t *testing.T) {
	db := newTestDB(t)

	for _, n := range []uint64{100, 101, 102} {
		if err := InsertBlock(db, n, "0xhash"); err != nil {
			t.Fatalf("InsertBlock(%d) failed: %v", n, err)
		}
		tr := Transfer{
			BlockNumber: n,
			LogIndex:    0,
			TxHash:      "0xtx" + string(rune('a'+n-100)),
			TokenSymbol: "USDC",
			From:        "0xfrom",
			To:          "0xto",
			RawAmount:   "1",
		}
		if err := InsertTransfer(db, tr); err != nil {
			t.Fatalf("InsertTransfer(%d) failed: %v", n, err)
		}
	}

	// Fork point is 100, so 101 and 102 are stale and must go.
	if err := DeleteAbove(db, 100); err != nil {
		t.Fatalf("DeleteAbove failed: %v", err)
	}

	for _, n := range []uint64{101, 102} {
		if _, ok, _ := GetBlockHash(db, n); ok {
			t.Fatalf("block %d survived DeleteAbove", n)
		}
	}
	if _, ok, _ := GetBlockHash(db, 100); !ok {
		t.Fatal("block 100 is the fork point and must survive DeleteAbove")
	}

	var transfersLeft int
	if err := db.QueryRow(`SELECT COUNT(*) FROM transfers`).Scan(&transfersLeft); err != nil {
		t.Fatalf("counting transfers failed: %v", err)
	}
	if transfersLeft != 1 {
		t.Fatalf("got %d transfers left, want 1 (only the fork-point block's)", transfersLeft)
	}
}

func TestPruneBlocksBelow(t *testing.T) {
	db := newTestDB(t)

	for _, n := range []uint64{98, 99, 100, 101} {
		if err := InsertBlock(db, n, "0xhash"); err != nil {
			t.Fatalf("InsertBlock(%d) failed: %v", n, err)
		}
	}

	// Finalized at 99: 98 and 99 can never reorg, so their hashes are dead weight.
	if err := PruneBlocksBelow(db, 99); err != nil {
		t.Fatalf("PruneBlocksBelow failed: %v", err)
	}

	for _, n := range []uint64{98, 99} {
		if _, ok, _ := GetBlockHash(db, n); ok {
			t.Fatalf("finalized block %d should have been pruned", n)
		}
	}
	for _, n := range []uint64{100, 101} {
		if _, ok, _ := GetBlockHash(db, n); !ok {
			t.Fatalf("unfinalized block %d must be kept", n)
		}
	}
}

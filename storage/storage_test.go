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

func TestRollback(t *testing.T) {
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
	// Simulate having already advanced the checkpoint to the (now stale) tip.
	if err := RecordBlock(db, 102, "0xhash"); err != nil {
		t.Fatalf("RecordBlock failed: %v", err)
	}

	// Fork point is 100, so 101 and 102 are stale and must go.
	if err := Rollback(db, 100, "0xfork"); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	for _, n := range []uint64{101, 102} {
		if _, ok, _ := GetBlockHash(db, n); ok {
			t.Fatalf("block %d survived Rollback", n)
		}
	}
	if _, ok, _ := GetBlockHash(db, 100); !ok {
		t.Fatal("block 100 is the fork point and must survive Rollback")
	}

	var transfersLeft int
	if err := db.QueryRow(`SELECT COUNT(*) FROM transfers`).Scan(&transfersLeft); err != nil {
		t.Fatalf("counting transfers failed: %v", err)
	}
	if transfersLeft != 1 {
		t.Fatalf("got %d transfers left, want 1 (only the fork-point block's)", transfersLeft)
	}

	// The checkpoint must move back too, atomically with the deletes - not
	// still point at 102, which no longer exists.
	blockNumber, blockHash, found, err := LoadCheckpoint(db)
	if err != nil {
		t.Fatalf("LoadCheckpoint failed: %v", err)
	}
	if !found || blockNumber != 100 || blockHash != "0xfork" {
		t.Fatalf("got (block=%d, hash=%q, found=%v), want (block=100, hash=%q, found=true)", blockNumber, blockHash, found, "0xfork")
	}
}

func TestLoadCheckpointOnFreshDB(t *testing.T) {
	db := newTestDB(t)

	_, _, found, err := LoadCheckpoint(db)
	if err != nil {
		t.Fatalf("LoadCheckpoint failed: %v", err)
	}
	if found {
		t.Fatal("got found=true on a database that has never recorded a block")
	}
}

func TestRecordBlockUpdatesCheckpointAtomically(t *testing.T) {
	db := newTestDB(t)

	if err := RecordBlock(db, 100, "0xaaa"); err != nil {
		t.Fatalf("RecordBlock failed: %v", err)
	}

	hash, ok, err := GetBlockHash(db, 100)
	if err != nil {
		t.Fatalf("GetBlockHash failed: %v", err)
	}
	if !ok || hash != "0xaaa" {
		t.Fatalf("block row: got (%q, %v), want (%q, true)", hash, ok, "0xaaa")
	}

	blockNumber, blockHash, found, err := LoadCheckpoint(db)
	if err != nil {
		t.Fatalf("LoadCheckpoint failed: %v", err)
	}
	if !found || blockNumber != 100 || blockHash != "0xaaa" {
		t.Fatalf("checkpoint: got (block=%d, hash=%q, found=%v), want (block=100, hash=%q, found=true)", blockNumber, blockHash, found, "0xaaa")
	}

	// A second block must advance the checkpoint, not add a second row -
	// CHECK(id = 1) is what enforces that at the schema level.
	if err := RecordBlock(db, 101, "0xbbb"); err != nil {
		t.Fatalf("second RecordBlock failed: %v", err)
	}
	blockNumber, blockHash, _, err = LoadCheckpoint(db)
	if err != nil {
		t.Fatalf("LoadCheckpoint failed: %v", err)
	}
	if blockNumber != 101 || blockHash != "0xbbb" {
		t.Fatalf("got (block=%d, hash=%q), want (block=101, hash=%q)", blockNumber, blockHash, "0xbbb")
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

func TestMarkFinalized(t *testing.T) {
	db := newTestDB(t)

	old := Transfer{
		BlockNumber: 100,
		LogIndex:    0,
		TxHash:      "0xold",
		TokenSymbol: "USDC",
		From:        "0xfrom",
		To:          "0xto",
		RawAmount:   "1",
		IsFinal:     false,
	}
	recent := Transfer{
		BlockNumber: 200,
		LogIndex:    0,
		TxHash:      "0xrecent",
		TokenSymbol: "USDC",
		From:        "0xfrom",
		To:          "0xto",
		RawAmount:   "1",
		IsFinal:     false,
	}
	if err := InsertTransfer(db, old); err != nil {
		t.Fatalf("InsertTransfer(old) failed: %v", err)
	}
	if err := InsertTransfer(db, recent); err != nil {
		t.Fatalf("InsertTransfer(recent) failed: %v", err)
	}

	// Finalized at 150: block 100 has caught up to finality, block 200 hasn't yet.
	flipped, err := MarkFinalized(db, 150)
	if err != nil {
		t.Fatalf("MarkFinalized failed: %v", err)
	}
	if flipped != 1 {
		t.Fatalf("got %d rows flipped, want 1 (only the block-100 transfer)", flipped)
	}

	var isFinal bool
	if err := db.QueryRow(`SELECT is_final FROM transfers WHERE tx_hash = ?`, old.TxHash).Scan(&isFinal); err != nil {
		t.Fatalf("reading back the old transfer failed: %v", err)
	}
	if !isFinal {
		t.Fatal("block-100 transfer should now be marked final")
	}

	if err := db.QueryRow(`SELECT is_final FROM transfers WHERE tx_hash = ?`, recent.TxHash).Scan(&isFinal); err != nil {
		t.Fatalf("reading back the recent transfer failed: %v", err)
	}
	if isFinal {
		t.Fatal("block-200 transfer isn't finalized yet and must not have been flipped")
	}
}

// A transfer already marked final shouldn't be counted again on a later
// sweep - MarkFinalized only touches rows still marked is_final = 0.
func TestMarkFinalizedIsNotRecountedOnRepeatSweep(t *testing.T) {
	db := newTestDB(t)

	tr := Transfer{
		BlockNumber: 100,
		LogIndex:    0,
		TxHash:      "0xtx",
		TokenSymbol: "USDC",
		From:        "0xfrom",
		To:          "0xto",
		RawAmount:   "1",
		IsFinal:     false,
	}
	if err := InsertTransfer(db, tr); err != nil {
		t.Fatalf("InsertTransfer failed: %v", err)
	}

	first, err := MarkFinalized(db, 150)
	if err != nil {
		t.Fatalf("first MarkFinalized failed: %v", err)
	}
	if first != 1 {
		t.Fatalf("got %d flipped on first sweep, want 1", first)
	}

	second, err := MarkFinalized(db, 200)
	if err != nil {
		t.Fatalf("second MarkFinalized failed: %v", err)
	}
	if second != 0 {
		t.Fatalf("got %d flipped on second sweep, want 0 (already final, nothing left to do)", second)
	}
}

func TestGetUnnotifiedFinalTransfers(t *testing.T) {
	db := newTestDB(t)

	finalNotNotified := Transfer{
		BlockNumber: 100, LogIndex: 0, TxHash: "0xfinal-unnotified",
		TokenSymbol: "USDC", From: "0xfrom", To: "0xto", RawAmount: "1", IsFinal: true,
	}
	notFinal := Transfer{
		BlockNumber: 200, LogIndex: 0, TxHash: "0xnot-final",
		TokenSymbol: "USDC", From: "0xfrom", To: "0xto", RawAmount: "1", IsFinal: false,
	}
	if err := InsertTransfer(db, finalNotNotified); err != nil {
		t.Fatalf("InsertTransfer(finalNotNotified) failed: %v", err)
	}
	if err := InsertTransfer(db, notFinal); err != nil {
		t.Fatalf("InsertTransfer(notFinal) failed: %v", err)
	}

	// A final transfer that's already been notified about shouldn't come
	// back either - insert it final, then mark it sent.
	finalAlreadyNotified := Transfer{
		BlockNumber: 300, LogIndex: 0, TxHash: "0xfinal-already-notified",
		TokenSymbol: "USDC", From: "0xfrom", To: "0xto", RawAmount: "1", IsFinal: true,
	}
	if err := InsertTransfer(db, finalAlreadyNotified); err != nil {
		t.Fatalf("InsertTransfer(finalAlreadyNotified) failed: %v", err)
	}
	if err := MarkWebhookSent(db, finalAlreadyNotified.TxHash, finalAlreadyNotified.LogIndex); err != nil {
		t.Fatalf("MarkWebhookSent failed: %v", err)
	}

	got, err := GetUnnotifiedFinalTransfers(db)
	if err != nil {
		t.Fatalf("GetUnnotifiedFinalTransfers failed: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d transfers, want 1 (only the final-and-unnotified one)", len(got))
	}
	if got[0].TxHash != finalNotNotified.TxHash {
		t.Fatalf("got tx hash %q, want %q", got[0].TxHash, finalNotNotified.TxHash)
	}
}

func TestMarkWebhookSent_RemovesFromUnnotifiedList(t *testing.T) {
	db := newTestDB(t)

	tr := Transfer{
		BlockNumber: 100, LogIndex: 5, TxHash: "0xtx",
		TokenSymbol: "USDC", From: "0xfrom", To: "0xto", RawAmount: "1", IsFinal: true,
	}
	if err := InsertTransfer(db, tr); err != nil {
		t.Fatalf("InsertTransfer failed: %v", err)
	}

	before, err := GetUnnotifiedFinalTransfers(db)
	if err != nil {
		t.Fatalf("GetUnnotifiedFinalTransfers failed: %v", err)
	}
	if len(before) != 1 {
		t.Fatalf("got %d unnotified before marking sent, want 1", len(before))
	}

	if err := MarkWebhookSent(db, tr.TxHash, tr.LogIndex); err != nil {
		t.Fatalf("MarkWebhookSent failed: %v", err)
	}

	after, err := GetUnnotifiedFinalTransfers(db)
	if err != nil {
		t.Fatalf("GetUnnotifiedFinalTransfers failed: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("got %d unnotified after marking sent, want 0", len(after))
	}
}

package chainsync

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"
	"path/filepath"
	"testing"

	"github.com/ayush18pop/surety/storage"
	"github.com/ethereum/go-ethereum/core/types"
)

// fakeHeaderFetcher stands in for a real chain client. A real mainnet reorg
// can't be triggered on demand to test against, so this lets a test build
// one by hand instead.
type fakeHeaderFetcher struct {
	headers map[uint64]*types.Header
}

func (f *fakeHeaderFetcher) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	n := number.Uint64()
	h, ok := f.headers[n]
	if !ok {
		return nil, fmt.Errorf("fake chain has no header at block %d", n)
	}
	return h, nil
}

// header builds a real *types.Header - its .Hash() is a genuine Keccak hash
// of its fields, not faked. variant only exists to make two headers at the
// same block number produce different hashes, simulating "this height looks
// different depending on which branch of the chain you're asking."
func header(number uint64, variant string) *types.Header {
	return &types.Header{
		Number: big.NewInt(int64(number)),
		Extra:  []byte(variant),
	}
}

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("storage.Open failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := storage.InitSchema(db); err != nil {
		t.Fatalf("storage.InitSchema failed: %v", err)
	}
	return db
}

func TestFindForkPoint_NoReorg(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// Chain and storage agree at every height - nothing to walk back through.
	fake := &fakeHeaderFetcher{headers: map[uint64]*types.Header{}}
	for n := uint64(100); n <= 105; n++ {
		h := header(n, "canonical")
		fake.headers[n] = h
		if err := storage.InsertBlock(db, n, h.Hash().Hex()); err != nil {
			t.Fatalf("InsertBlock(%d) failed: %v", n, err)
		}
	}

	forkPoint, err := FindForkPoint(fake, ctx, db, 105, 95)
	if err != nil {
		t.Fatalf("FindForkPoint failed: %v", err)
	}
	if forkPoint != 105 {
		t.Fatalf("got fork point %d, want 105 (no reorg, should match immediately)", forkPoint)
	}
}

func TestFindForkPoint_ShallowReorg(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	fake := &fakeHeaderFetcher{headers: map[uint64]*types.Header{}}

	// 100-102 are unaffected: chain and storage still agree.
	for n := uint64(100); n <= 102; n++ {
		h := header(n, "canonical")
		fake.headers[n] = h
		if err := storage.InsertBlock(db, n, h.Hash().Hex()); err != nil {
			t.Fatalf("InsertBlock(%d) failed: %v", n, err)
		}
	}

	// 103-105: storage still holds the orphaned hashes from before the
	// reorg, but the fake chain now reports different headers at those same
	// heights - exactly the state a real reorg leaves behind.
	for n := uint64(103); n <= 105; n++ {
		orphaned := header(n, "orphaned")
		if err := storage.InsertBlock(db, n, orphaned.Hash().Hex()); err != nil {
			t.Fatalf("InsertBlock(%d) failed: %v", n, err)
		}
		fake.headers[n] = header(n, "canonical-after-reorg")
	}

	forkPoint, err := FindForkPoint(fake, ctx, db, 105, 95)
	if err != nil {
		t.Fatalf("FindForkPoint failed: %v", err)
	}
	if forkPoint != 102 {
		t.Fatalf("got fork point %d, want 102 (last height both chains agree on)", forkPoint)
	}
}

func TestFindForkPoint_NoStoredHistory_ReturnsFinalizedFloor(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// Nothing recorded in storage at all - every height from 100 down to the
	// floor is a miss, so the walk should fall all the way through.
	fake := &fakeHeaderFetcher{headers: map[uint64]*types.Header{
		100: header(100, "whatever"),
	}}

	forkPoint, err := FindForkPoint(fake, ctx, db, 100, 90)
	if err != nil {
		t.Fatalf("FindForkPoint failed: %v", err)
	}
	if forkPoint != 90 {
		t.Fatalf("got fork point %d, want 90 (the finalized floor, since nothing was ever stored to match against)", forkPoint)
	}
}

// The empty fake here is deliberate: if FindForkPoint made any chain call in
// this branch, the fake would return an error (no header registered) and the
// test would fail - proving the early return happens before ever asking the
// chain anything.
func TestFindForkPoint_AlreadyAtOrBelowFinality(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	fake := &fakeHeaderFetcher{headers: map[uint64]*types.Header{}}

	forkPoint, err := FindForkPoint(fake, ctx, db, 90, 95)
	if err != nil {
		t.Fatalf("FindForkPoint failed: %v", err)
	}
	if forkPoint != 90 {
		t.Fatalf("got fork point %d, want 90 (from is already at/below finality)", forkPoint)
	}
}

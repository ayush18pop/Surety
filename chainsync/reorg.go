package chainsync

import (
	"context"
	"database/sql"
	"math/big"

	"github.com/ayush18pop/surety/storage"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

// GetFinalizedBlock returns the highest block number that Ethereum's consensus
// layer has cryptoeconomically finalized, not a guessed confirmation count.
// rpc.FinalizedBlockNumber is a negative sentinel (-3) that ethclient turns into
// the literal "finalized" tag over JSON-RPC.
func GetFinalizedBlock(client *ethclient.Client, ctx context.Context) (uint64, error) {
	header, err := client.HeaderByNumber(ctx, big.NewInt(rpc.FinalizedBlockNumber.Int64()))
	if err != nil {
		return 0, err
	}
	return header.Number.Uint64(), nil
}

// FindForkPoint walks backwards from `from`, comparing the hash we recorded at
// each height against the hash the live chain reports there now. The first
// height where they agree is the fork point: the last block both chains share.
//
// A reorg at depth N invalidates every block above N too (each block's hash
// feeds into its child's), so the divergence is always a contiguous run at the
// top - which is why walking down and stopping at the first match is enough.
//
// finalizedBlock is the floor, and it's a guarantee rather than a guess:
// finalized blocks cannot reorg, so a match is certain at or before it.
func FindForkPoint(client *ethclient.Client, ctx context.Context, db *sql.DB, from uint64, finalizedBlock uint64) (uint64, error) {
	if from <= finalizedBlock {
		return from, nil // already at or below finality, nothing could have reorged
	}

	for n := from; n > finalizedBlock; n-- {
		storedHash, ok, err := storage.GetBlockHash(db, n)
		if err != nil {
			return 0, err
		}
		if !ok {
			continue // never recorded this height, nothing to compare or discard
		}

		header, err := client.HeaderByNumber(ctx, big.NewInt(int64(n)))
		if err != nil {
			return 0, err
		}
		if header.Hash().Hex() == storedHash {
			return n, nil
		}
	}

	return finalizedBlock, nil
}

// HandleReorg finds the fork point, discards everything above it (and moves
// the checkpoint back to it, atomically with those deletes), and returns the
// checkpoint to resume from. The caller re-indexes forward from there on the
// now-canonical chain.
func HandleReorg(client *ethclient.Client, ctx context.Context, db *sql.DB, cp Checkpoint, finalizedBlock uint64) (Checkpoint, error) {
	forkPoint, err := FindForkPoint(client, ctx, db, cp.BlockNumber, finalizedBlock)
	if err != nil {
		return Checkpoint{}, err
	}

	// The fork point's hash usually comes straight from our own table, but it
	// may have been pruned (it can be the finalized block itself), so fall
	// back to asking the chain. This has to happen before Rollback, since
	// Rollback needs the hash to write the new checkpoint in the same
	// transaction as the deletes.
	hash, ok, err := storage.GetBlockHash(db, forkPoint)
	if err != nil {
		return Checkpoint{}, err
	}
	if !ok {
		header, err := client.HeaderByNumber(ctx, big.NewInt(int64(forkPoint)))
		if err != nil {
			return Checkpoint{}, err
		}
		hash = header.Hash().Hex()
	}

	if err := storage.Rollback(db, forkPoint, hash); err != nil {
		return Checkpoint{}, err
	}

	return Checkpoint{BlockNumber: forkPoint, BlockHash: hash}, nil
}

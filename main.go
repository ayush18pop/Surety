package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/ayush18pop/surety/chainsync"
	"github.com/ayush18pop/surety/storage"
	"github.com/ayush18pop/surety/tokenwatch"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/joho/godotenv"
)

// watchedTokens maps each token contract to its symbol/decimals, a map, not a
// slice, because tokenwatch needs to know which decimals apply to which token
// when a batch of logs comes back from more than one contract at once.
var watchedTokens = map[common.Address]tokenwatch.TokenInfo{
	common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48"): {Symbol: "USDC", Decimals: 6},
}

// fatal logs an error with context and exits. Only main() gets to do this -
// everything below it returns errors instead of killing the process, since a
// library function calling os.Exit takes that decision away from its caller.
func fatal(msg string, err error) {
	slog.Error(msg, "error", err)
	os.Exit(1)
}

func main() {
	// Text output for now - readable while actively developing. Swapping to
	// JSON (for a log aggregator to parse) is this one line:
	// slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))

	if err := godotenv.Load(); err != nil {
		fatal("loading .env", err)
	}

	rpcURL := os.Getenv("ETH_MAINNET_RPC")

	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		fatal("dialing RPC", err)
	}
	defer client.Close()

	db, err := storage.Open("surety.db")
	if err != nil {
		fatal("opening database", err)
	}
	defer db.Close()

	if err := storage.InitSchema(db); err != nil {
		fatal("initializing schema", err)
	}

	ctx := context.Background()

	blockNumber, blockHash, found, err := storage.LoadCheckpoint(db)
	if err != nil {
		fatal("loading checkpoint", err)
	}
	cp := chainsync.Checkpoint{BlockNumber: blockNumber, BlockHash: blockHash}
	if !found {
		// Fresh database, never recorded a block: BlockNumber stays 0, which
		// the loop below reads as "seed from the current chain tip."
		slog.Info("no checkpoint found, starting from the current chain tip")
	}

	for {
		latest, _ := client.HeaderByNumber(ctx, nil)
		latestNum := latest.Number.Uint64()
		if cp.BlockNumber == 0 {
			cp.BlockNumber = latestNum
			continue
		}

		finalizedBlock, err := chainsync.GetFinalizedBlock(client, ctx)
		if err != nil {
			fatal("fetching finalized block", err)
		}

		if latestNum > cp.BlockNumber {
			slog.Info("catching up", "from_block", cp.BlockNumber+1, "to_block", latestNum)
			for blockNum := cp.BlockNumber + 1; blockNum <= latestNum; blockNum++ {
				slog.Debug("processing block", "block_number", blockNum)
				hash, parentHash, err := chainsync.GetBlockHashes(client, ctx, blockNum)
				if err != nil {
					fatal("fetching block hashes", err)
				}

				// cp.BlockHash is empty right after a fresh start (no prior
				// block to compare against yet) - only check continuity once
				// there's a real previous hash on record.
				if cp.BlockHash != "" && parentHash != cp.BlockHash {
					slog.Warn("reorg detected",
						"block_number", blockNum,
						"got_parent_hash", parentHash,
						"expected_parent_hash", cp.BlockHash,
						"last_processed_block", cp.BlockNumber,
					)

					// HandleReorg's Rollback already moves the checkpoint back
					// to the fork point atomically, in the same transaction as
					// the deletes - nothing further to save here.
					newCp, err := chainsync.HandleReorg(client, ctx, db, cp, finalizedBlock)
					if err != nil {
						fatal("handling reorg", err)
					}
					slog.Info("rolled back to fork point", "block_number", newCp.BlockNumber)

					cp = newCp
					break // restart the catch-up from the fork point
				}

				// Don't advance the checkpoint past a block whose transfers we
				// failed to read - a transient RPC error would otherwise skip
				// that block permanently. Leaving the checkpoint where it is
				// means the next tick retries this same block, which the
				// idempotent inserts make safe.
				if err := tokenwatch.CheckTransfers(client, ctx, blockNum, watchedTokens, db, finalizedBlock); err != nil {
					slog.Error("checking transfers, retrying next tick", "block_number", blockNum, "error", err)
					break
				}
				// RecordBlock writes the block hash and advances the checkpoint
				// in one transaction - a crash between the two would otherwise
				// leave them disagreeing about how far we've actually gotten.
				if err := storage.RecordBlock(db, blockNum, hash); err != nil {
					fatal("recording block", err)
				}
				cp = chainsync.Checkpoint{BlockNumber: blockNum, BlockHash: hash}
			}
		}

		// Finalized blocks can't reorg, so their hashes will never be compared
		// against again.
		if err := storage.PruneBlocksBelow(db, finalizedBlock); err != nil {
			fatal("pruning finalized blocks", err)
		}

		time.Sleep(12 * time.Second)

	}

}

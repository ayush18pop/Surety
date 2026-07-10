package main

import (
	"context"
	"fmt"
	"log"
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

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatal(err)
	}

	rpcURL := os.Getenv("ETH_MAINNET_RPC")

	client, err := ethclient.Dial(rpcURL)

	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	db, err := storage.Open("surety.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := storage.InitSchema(db); err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	cp, err := chainsync.LoadCheckpoint("checkpoint.json")

	if err != nil {
		chainsync.SaveCheckpoint(chainsync.Checkpoint{}, client, ctx)
	}

	for {
		latest, _ := client.BlockByNumber(ctx, nil)
		latestNum := latest.NumberU64()
		if cp.BlockNumber == 0 {
			cp.BlockNumber = latestNum
			continue
		}

		finalizedBlock, err := chainsync.GetFinalizedBlock(client, ctx)
		if err != nil {
			log.Fatal(err)
		}

		if latestNum > cp.BlockNumber {
			fmt.Printf("Need to process blocks %d to %d\n", cp.BlockNumber+1, latestNum)
			for blockNum := cp.BlockNumber + 1; blockNum <= latestNum; blockNum++ {
				fmt.Println("Processing block:", blockNum)
				hash, parentHash, err := chainsync.ProcessBlock(client, ctx, blockNum)
				if err != nil {
					log.Fatal(err)
				}

				// cp.BlockHash is empty right after a fresh start (no prior
				// block to compare against yet) - only check continuity once
				// there's a real previous hash on record.
				if cp.BlockHash != "" && parentHash != cp.BlockHash {
					fmt.Printf(
						"REORG DETECTED: block %d's parent is %s, but checkpoint expected %s (last processed: block %d)\n",
						blockNum, parentHash, cp.BlockHash, cp.BlockNumber,
					)

					newCp, err := chainsync.HandleReorg(client, ctx, db, cp, finalizedBlock)
					if err != nil {
						log.Fatal(err)
					}
					fmt.Printf("rolled back to fork point: block %d\n", newCp.BlockNumber)

					cp = newCp
					chainsync.SaveCheckpoint(cp, client, ctx)
					break // restart the catch-up from the fork point
				}

				// Don't advance the checkpoint past a block whose transfers we
				// failed to read - a transient RPC error would otherwise skip
				// that block permanently. Leaving the checkpoint where it is
				// means the next tick retries this same block, which the
				// idempotent inserts make safe.
				if err := tokenwatch.CheckTransfers(client, ctx, blockNum, watchedTokens, db, finalizedBlock); err != nil {
					fmt.Printf("CheckTransfers failed on block %d, retrying next tick: %v\n", blockNum, err)
					break
				}
				if err := storage.InsertBlock(db, blockNum, hash); err != nil {
					log.Fatal(err)
				}
				cp = chainsync.Checkpoint{BlockNumber: blockNum, BlockHash: hash}
				chainsync.SaveCheckpoint(cp, client, ctx)
			}
		}

		// Finalized blocks can't reorg, so their hashes will never be compared
		// against again.
		if err := storage.PruneBlocksBelow(db, finalizedBlock); err != nil {
			log.Fatal(err)
		}

		time.Sleep(12 * time.Second)

	}

}

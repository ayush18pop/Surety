package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ayush18pop/done/chainsync"
	"github.com/ayush18pop/done/storage"
	"github.com/ayush18pop/done/tokenwatch"
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

	db, err := storage.Open("done.db")
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
				}

				if err := tokenwatch.CheckTransfers(client, ctx, blockNum, watchedTokens, db); err != nil {
					fmt.Println("CheckTransfers failed:", err)
				}
				cp = chainsync.Checkpoint{BlockNumber: blockNum, BlockHash: hash}
				chainsync.SaveCheckpoint(cp, client, ctx)
			}
		}
		time.Sleep(12 * time.Second)

	}

}

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ayush18pop/done/chainsync"
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

	ctx := context.Background()

	lastProcessed, err := chainsync.LoadCheckpoint("checkpoint.txt")

	if err != nil {
		chainsync.SaveCheckpoint(0, client, ctx)
	}

	for {
		latest, _ := client.BlockByNumber(ctx, nil)
		latestNum := latest.NumberU64()
		if lastProcessed == 0 {
			lastProcessed = latestNum
			continue
		}

		if latestNum > lastProcessed {
			fmt.Printf("Need to process blocks %d to %d\n", lastProcessed+1, latestNum)
			for blockNum := lastProcessed + 1; blockNum <= latestNum; blockNum++ {
				fmt.Println("Processing block:", blockNum)
				err = chainsync.ProcessBlock(client, ctx, blockNum)
				if err != nil {
					log.Fatal(err)
				}
				if err := tokenwatch.CheckTransfers(client, ctx, blockNum, watchedTokens); err != nil {
					fmt.Println("CheckTransfers failed:", err)
				}
				lastProcessed = blockNum
				chainsync.SaveCheckpoint(lastProcessed, client, ctx)
			}
		}
		time.Sleep(12 * time.Second)

	}

}

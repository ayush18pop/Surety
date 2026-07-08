package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ayush18pop/done/chainsync"
	"github.com/ayush18pop/done/tokenwatch"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/joho/godotenv"
)

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
				if err := tokenwatch.CheckUSDCTransfers(client, ctx, blockNum); err != nil {
					fmt.Println("CheckUSDCTransfers failed:", err)
				}
				lastProcessed = blockNum
				chainsync.SaveCheckpoint(lastProcessed, client, ctx)
			}
		}
		time.Sleep(12 * time.Second)

	}

}

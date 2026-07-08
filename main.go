package main

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"os"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/joho/godotenv"
)

func processBlock(client *ethclient.Client, ctx context.Context, blockNum uint64) error {
	b, err := client.BlockByNumber(ctx, big.NewInt(int64(blockNum)))
	if err != nil {
		return err
	}
	fmt.Println("Block number:", b.Number())
	fmt.Println("Block hash:", b.Hash())
	fmt.Println("Timestamp:", b.Time())
	fmt.Println("Transaction ", len(b.Transactions()))
	for i, tx := range b.Transactions() {
		fmt.Println("Index:", i)
		if tx.Protected() {
			signer := types.LatestSignerForChainID(tx.ChainId())
			from, err := types.Sender(signer, tx)
			if err != nil {
				return err
			}
			fmt.Println("tx is form : ", from)
			fmt.Println("chainid:", tx.ChainId())
		} else {
			signer := types.HomesteadSigner{}
			from, err := types.Sender(signer, tx)
			if err != nil {
				return err
			}
			fmt.Println("tx is form : ", from)
			fmt.Println("chainid:", tx.ChainId())

		}
		fmt.Println("Nonce:", tx.Nonce())
		fmt.Println("To:", tx.To())
		fmt.Println("Value:", tx.Value())
		fmt.Println("Gas:", tx.Gas())
		fmt.Println("Gas price:", tx.GasPrice())
		fmt.Println("Data:", tx.Data())
	}
	fmt.Println("transactions : ")
	fmt.Println("Gas used:", b.GasUsed())
	fmt.Println("Gas limit:", b.GasLimit())
	return nil
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
	checkpt, err := os.ReadFile("checkpoint.txt")
	if len(checkpt) == 0 {
		b, _ := client.BlockByNumber(ctx, nil)
		bnum := []byte(strconv.FormatUint(b.NumberU64(), 10))
		os.WriteFile("checkpoint.txt", bnum, 0666)
	}
	checkpt, err = os.ReadFile("checkpoint.txt")
	lastProcessed, err := strconv.ParseUint(string(checkpt), 10, 64)

	if err != nil {
		log.Fatal(err)
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
				err = processBlock(client, ctx, blockNum)
				if err != nil {
					log.Fatal(err)
				}
				lastProcessed = blockNum
				data := []byte(strconv.FormatUint(lastProcessed, 10))
				fileWriteErr := os.WriteFile("checkpoint.txt", data, 0666)
				if fileWriteErr != nil {
					panic(fileWriteErr)
				}
			}
		}
		time.Sleep(12 * time.Second)

	}

}

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/joho/godotenv"
)


var usdc = common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")

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

func loadCheckpoint(fileName string) (uint64, error) {
	checkpt, err := os.ReadFile(fileName)
	if err != nil {
		log.Fatal("failed to load checkpoint!")
	}
	if len(checkpt) == 0 {
		return 0, fmt.Errorf("empty checkpoint file!")
	}
	lastProcessed, err := strconv.ParseUint(string(checkpt), 10, 64)
	return lastProcessed, err
}

func saveCheckpoint(pt uint64, client *ethclient.Client, ctx context.Context) error {
	if pt == 0 {
		b, _ := client.BlockByNumber(ctx, nil)
		bnum := []byte(strconv.FormatUint(b.NumberU64(), 10))
		err := os.WriteFile("checkpoint.txt", bnum, 0666)
		if err != nil {
			log.Fatal(errors.Join(fmt.Errorf("Write failed: "), err))
		}
	} else {
		data := []byte(strconv.FormatUint(pt, 10))
		fileWriteErr := os.WriteFile("checkpoint.txt", data, 0666)
		if fileWriteErr != nil {
			log.Fatal(errors.Join(fmt.Errorf("Write failed: "), fileWriteErr))
		}
	}

	return nil
}

func checkUsdcTransfers(client *ethclient.Client, ctx context.Context, blockNum uint64) error {
	transferSig := crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))

	query := ethereum.FilterQuery{
		FromBlock: big.NewInt(int64(blockNum)),
		ToBlock:   big.NewInt(int64(blockNum)),
		Addresses: []common.Address{usdc},
		Topics:    [][]common.Hash{{transferSig}},
	}

	logs, err := client.FilterLogs(ctx, query)
	if err != nil {
		return err
	}
	var builder strings.Builder

	for _, vLog := range logs {
		from := common.HexToAddress(vLog.Topics[1].Hex())
		to := common.HexToAddress(vLog.Topics[2].Hex())

		amount := new(big.Int).SetBytes(vLog.Data)

		humanAmount := new(big.Rat).SetFrac(amount, big.NewInt(1_000_000))

		builder.WriteString("============================================================\n")
		builder.WriteString("USDC Transfer\n")
		builder.WriteString("============================================================\n")

		builder.WriteString(fmt.Sprintf("Block      : %d\n", vLog.BlockNumber))
		builder.WriteString(fmt.Sprintf("Tx Hash    : %s\n", vLog.TxHash.Hex()))
		builder.WriteString(fmt.Sprintf("Log Index  : %d\n\n", vLog.Index))

		builder.WriteString(fmt.Sprintf("From       : %s\n", from.Hex()))
		builder.WriteString(fmt.Sprintf("To         : %s\n", to.Hex()))
		builder.WriteString(fmt.Sprintf("Amount     : %s USDC\n", humanAmount.FloatString(6)))
		builder.WriteString(fmt.Sprintf("Raw Amount : %s\n", amount.String()))

		builder.WriteString("\n------------------------------------------------------------\n\n")
	}

	err = os.WriteFile("usdc_transfers.txt", []byte(builder.String()), 0644)
	if err != nil {
		return err
	}
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

	lastProcessed, err := loadCheckpoint("checkpoint.txt")
	
	if err != nil {
		saveCheckpoint(0, client, ctx)
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
				if err := checkUsdcTransfers(client, ctx, blockNum); err != nil {
					fmt.Println("checkUsdcTransfers failed:", err)
				}
				lastProcessed = blockNum
				saveCheckpoint(lastProcessed, client, ctx)
			}
		}
		time.Sleep(12 * time.Second)

	}

}

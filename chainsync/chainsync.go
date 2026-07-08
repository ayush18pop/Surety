package chainsync

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"strconv"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

func ProcessBlock(client *ethclient.Client, ctx context.Context, blockNum uint64) error {
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

func LoadCheckpoint(fileName string) (uint64, error) {
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

func SaveCheckpoint(pt uint64, client *ethclient.Client, ctx context.Context) error {
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

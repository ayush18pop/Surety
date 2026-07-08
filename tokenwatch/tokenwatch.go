package tokenwatch

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

var usdc = common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")

func CheckUSDCTransfers(client *ethclient.Client, ctx context.Context, blockNum uint64) error {
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

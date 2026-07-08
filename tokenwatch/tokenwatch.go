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

type TokenInfo struct {
	Symbol   string
	Decimals int64
}

func CheckTransfers(client *ethclient.Client, ctx context.Context, blockNum uint64, tokens map[common.Address]TokenInfo) error {
	transferSig := crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))

	addresses := make([]common.Address, 0, len(tokens))
	for addr := range tokens {
		addresses = append(addresses, addr)
	}

	query := ethereum.FilterQuery{
		FromBlock: big.NewInt(int64(blockNum)),
		ToBlock:   big.NewInt(int64(blockNum)),
		Addresses: addresses,
		Topics:    [][]common.Hash{{transferSig}},
	}

	logs, err := client.FilterLogs(ctx, query)
	if err != nil {
		return err
	}
	var builder strings.Builder

	for _, vLog := range logs {
		info := tokens[vLog.Address] // whichever token actually emitted this log, not a flat assumption

		from := common.HexToAddress(vLog.Topics[1].Hex())
		to := common.HexToAddress(vLog.Topics[2].Hex())

		amount := new(big.Int).SetBytes(vLog.Data)

		divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(info.Decimals), nil)
		humanAmount := new(big.Rat).SetFrac(amount, divisor)

		builder.WriteString("============================================================\n")
		fmt.Fprintf(&builder, "%s Transfer\n", info.Symbol)
		builder.WriteString("============================================================\n")

		fmt.Fprintf(&builder, "Block      : %d\n", vLog.BlockNumber)
		fmt.Fprintf(&builder, "Tx Hash    : %s\n", vLog.TxHash.Hex())
		fmt.Fprintf(&builder, "Log Index  : %d\n\n", vLog.Index)

		fmt.Fprintf(&builder, "From       : %s\n", from.Hex())
		fmt.Fprintf(&builder, "To         : %s\n", to.Hex())
		fmt.Fprintf(&builder, "Amount     : %s %s\n", humanAmount.FloatString(int(info.Decimals)), info.Symbol)
		fmt.Fprintf(&builder, "Raw Amount : %s\n", amount.String())

		builder.WriteString("\n------------------------------------------------------------\n\n")
	}

	err = os.WriteFile("usdc_transfers.txt", []byte(builder.String()), 0644)
	if err != nil {
		return err
	}
	return nil
}

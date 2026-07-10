package tokenwatch

import (
	"context"
	"database/sql"
	"log/slog"
	"math/big"

	"github.com/ayush18pop/surety/storage"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

type TokenInfo struct {
	Symbol   string
	Decimals int64
}

// finalizedBlock is passed in rather than fetched here: the caller already
// needs it once per polling tick, and looking it up again per block was an
// extra RPC round-trip for a value that barely moves.
func CheckTransfers(client *ethclient.Client, ctx context.Context, blockNum uint64, tokens map[common.Address]TokenInfo, db *sql.DB, finalizedBlock uint64) error {
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

	for _, vLog := range logs {
		info := tokens[vLog.Address] // whichever token actually emitted this log, not a flat assumption

		final := vLog.BlockNumber <= finalizedBlock

		from := common.HexToAddress(vLog.Topics[1].Hex())
		to := common.HexToAddress(vLog.Topics[2].Hex())

		amount := new(big.Int).SetBytes(vLog.Data)

		t := storage.Transfer{
			BlockNumber: vLog.BlockNumber,
			LogIndex:    vLog.Index,
			TxHash:      vLog.TxHash.Hex(),
			TokenSymbol: info.Symbol,
			From:        from.Hex(),
			To:          to.Hex(),
			RawAmount:   amount.String(),
			IsFinal:     final,
		}

		if err := storage.InsertTransfer(db, t); err != nil {
			return err
		}

		slog.Info("stored transfer",
			"block_number", t.BlockNumber,
			"tx_hash", t.TxHash,
			"log_index", t.LogIndex,
			"token", t.TokenSymbol,
			"from", t.From,
			"to", t.To,
			"final", t.IsFinal,
		)
	}

	return nil
}

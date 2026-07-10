package chainsync

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/ethclient"
)

// GetBlockHashes returns a block's own hash and its parent's hash.
//
// The caller needs both: the parent hash to check continuity against the last
// saved checkpoint, and the block's own hash to become the next checkpoint.
//
// This reads the header rather than the full block. Both values live in the
// header, so pulling transaction bodies over the wire would be wasted bandwidth.
func GetBlockHashes(client *ethclient.Client, ctx context.Context, blockNum uint64) (hash string, parentHash string, err error) {
	header, err := client.HeaderByNumber(ctx, big.NewInt(int64(blockNum)))
	if err != nil {
		return "", "", err
	}
	return header.Hash().Hex(), header.ParentHash.Hex(), nil
}

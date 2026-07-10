package chainsync

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/ethclient"
)

// Checkpoint is the two values needed to detect a reorg: not just how far
// we've processed, but the hash of the block we last trusted at that position,
// so a future block's ParentHash can be checked against it.
type Checkpoint struct {
	BlockNumber uint64 `json:"block_number"`
	BlockHash   string `json:"block_hash"`
}

// LoadCheckpoint reads and parses the JSON checkpoint file. A missing file
// returns an error rather than crashing the process - a fresh clone has no
// checkpoint yet, which is expected, and the caller seeds a new one.
func LoadCheckpoint(fileName string) (Checkpoint, error) {
	data, err := os.ReadFile(fileName)
	if err != nil {
		return Checkpoint{}, err
	}
	if len(data) == 0 {
		return Checkpoint{}, fmt.Errorf("empty checkpoint file")
	}
	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return Checkpoint{}, err
	}
	return cp, nil
}

// SaveCheckpoint writes the checkpoint to disk. A zero BlockNumber means
// "start from wherever the chain is now", so the current tip is fetched and
// recorded instead.
func SaveCheckpoint(cp Checkpoint, client *ethclient.Client, ctx context.Context) error {
	if cp.BlockNumber == 0 {
		header, err := client.HeaderByNumber(ctx, nil)
		if err != nil {
			return err
		}
		cp = Checkpoint{BlockNumber: header.Number.Uint64(), BlockHash: header.Hash().Hex()}
	}

	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile("checkpoint.json", data, 0666); err != nil {
		return fmt.Errorf("writing checkpoint: %w", err)
	}

	return nil
}

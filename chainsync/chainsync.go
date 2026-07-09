package chainsync

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"

	"github.com/ayush18pop/done/storage"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

// Checkpoint is the two values needed to detect a reorg later: not just how
// far we've processed, but the hash of the block we last trusted at that
// position, so a future block's ParentHash can be checked against it.
type Checkpoint struct {
	BlockNumber uint64 `json:"block_number"`
	BlockHash   string `json:"block_hash"`
}

// GetFinalizedBlock returns the highest block number that Ethereum's consensus
// layer has cryptoeconomically finalized, not a guessed confirmation count.
// rpc.FinalizedBlockNumber is a negative sentinel (-3) that ethclient turns into
// the literal "finalized" tag over JSON-RPC.
func GetFinalizedBlock(client *ethclient.Client, ctx context.Context) (uint64, error) {
	header, err := client.HeaderByNumber(ctx, big.NewInt(rpc.FinalizedBlockNumber.Int64()))
	if err != nil {
		return 0, err
	}
	return header.Number.Uint64(), nil
}

// FindForkPoint walks backwards from `from`, comparing the hash we recorded at
// each height against the hash the live chain reports there now. The first
// height where they agree is the fork point: the last block both chains share.
//
// A reorg at depth N invalidates every block above N too (each block's hash
// feeds into its child's), so the divergence is always a contiguous run at the
// top - which is why walking down and stopping at the first match is enough.
//
// finalizedBlock is the floor, and it's a guarantee rather than a guess:
// finalized blocks cannot reorg, so a match is certain at or before it.
func FindForkPoint(client *ethclient.Client, ctx context.Context, db *sql.DB, from uint64, finalizedBlock uint64) (uint64, error) {
	if from <= finalizedBlock {
		return from, nil // already at or below finality, nothing could have reorged
	}

	for n := from; n > finalizedBlock; n-- {
		storedHash, ok, err := storage.GetBlockHash(db, n)
		if err != nil {
			return 0, err
		}
		if !ok {
			continue // never recorded this height, nothing to compare or discard
		}

		header, err := client.HeaderByNumber(ctx, big.NewInt(int64(n)))
		if err != nil {
			return 0, err
		}
		if header.Hash().Hex() == storedHash {
			return n, nil
		}
	}

	return finalizedBlock, nil
}

// HandleReorg finds the fork point, discards everything above it, and returns
// the checkpoint to resume from. The caller re-indexes forward from there on
// the now-canonical chain.
func HandleReorg(client *ethclient.Client, ctx context.Context, db *sql.DB, cp Checkpoint, finalizedBlock uint64) (Checkpoint, error) {
	forkPoint, err := FindForkPoint(client, ctx, db, cp.BlockNumber, finalizedBlock)
	if err != nil {
		return Checkpoint{}, err
	}

	if err := storage.DeleteAbove(db, forkPoint); err != nil {
		return Checkpoint{}, err
	}

	// The fork point's hash usually comes straight from our own table, but it
	// may have been pruned (it can be the finalized block itself), so fall
	// back to asking the chain.
	hash, ok, err := storage.GetBlockHash(db, forkPoint)
	if err != nil {
		return Checkpoint{}, err
	}
	if !ok {
		header, err := client.HeaderByNumber(ctx, big.NewInt(int64(forkPoint)))
		if err != nil {
			return Checkpoint{}, err
		}
		hash = header.Hash().Hex()
	}

	return Checkpoint{BlockNumber: forkPoint, BlockHash: hash}, nil
}

// ProcessBlock returns the block's own hash and its parent's hash, so callers
// (the polling loop) can both save the checkpoint and check continuity
// against the previously saved checkpoint without a second, wasted RPC call
// to fetch a block they already have.
func ProcessBlock(client *ethclient.Client, ctx context.Context, blockNum uint64) (hash string, parentHash string, err error) {
	b, err := client.BlockByNumber(ctx, big.NewInt(int64(blockNum)))
	if err != nil {
		return "", "", err
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
				return "", "", err
			}
			fmt.Println("tx is form : ", from)
			fmt.Println("chainid:", tx.ChainId())
		} else {
			signer := types.HomesteadSigner{}
			from, err := types.Sender(signer, tx)
			if err != nil {
				return "", "", err
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
	return b.Hash().Hex(), b.ParentHash().Hex(), nil
}

// LoadCheckpoint reads and parses the JSON checkpoint file. Unlike the old
// version, a missing file returns an error instead of crashing the process —
// a fresh checkout with no checkpoint.json yet is an expected, recoverable
// case (main.go already handles it by seeding a new one via SaveCheckpoint),
// not a fatal one.
func LoadCheckpoint(fileName string) (Checkpoint, error) {
	data, err := os.ReadFile(fileName)
	if err != nil {
		return Checkpoint{}, err
	}
	if len(data) == 0 {
		return Checkpoint{}, fmt.Errorf("empty checkpoint file!")
	}
	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return Checkpoint{}, err
	}
	return cp, nil
}

func SaveCheckpoint(cp Checkpoint, client *ethclient.Client, ctx context.Context) error {
	if cp.BlockNumber == 0 {
		b, err := client.BlockByNumber(ctx, nil)
		if err != nil {
			return err
		}
		cp = Checkpoint{BlockNumber: b.NumberU64(), BlockHash: b.Hash().Hex()}
	}

	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile("checkpoint.json", data, 0666); err != nil {
		log.Fatal(fmt.Errorf("write failed: %w", err))
	}

	return nil
}

package cmd

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	cmtproto "github.com/cometbft/cometbft/proto/tendermint/store"
	db "github.com/cosmos/cosmos-db"
	proto "github.com/cosmos/gogoproto/proto"
	"golang.org/x/sync/errgroup"
)

type PrefixAndSplitter struct {
	prefix   string
	splitter heightParser
}

func (p PrefixAndSplitter) String() string {
	return p.prefix
}

// - Block headers (keys H:<HEIGHT>)
// - Commit information (keys C:<HEIGHT>)
// - ExtendedCommit information (keys EC:<HEIGHT>)
// - SeenCommit (keys SC:<HEIGHT>)
// - BlockPartKey (keys P:<HEIGHT>:<PART INDEX>)
// See https://github.com/cometbft/cometbft/blob/4591ef97ce5de702db7d6a3bbcb960ecf635fd76/store/db_key_layout.go#L38
// https://github.com/cometbft/cometbft/blob/4591ef97ce5de702db7d6a3bbcb960ecf635fd76/store/db_key_layout.go#L68
// for confirmation of this
// TODO: see if we can import that as consts?
var blockKeyInfos = []PrefixAndSplitter{
	{"H:", asciiHeightParser},         // block headers
	{"C:", asciiHeightParser},         // commit info
	{"EC:", asciiHeightParser},        // extended commits
	{"SC:", asciiHeightParser},        // seen commits
	{"P:", asciiHeightParserTwoParts}, // block parts
}

var stateKeyInfos = []string{
	"abciResponsesKey:",
	"consensusParamsKey:",
}

var appKeyInfos = []string{
	"s/",
}

func pruneBlockStore(blockStoreDB db.DB, pruneHeight uint64) error {

	storeState, err := blockStoreDB.Get([]byte("blockStore"))
	if err != nil {
		fmt.Println(err)
	}

	var bsj cmtproto.BlockStoreState

	if err := proto.Unmarshal(storeState, &bsj); err != nil {
		panic(fmt.Sprintf("Could not unmarshal bytes: %X", storeState))
	}

	base := bsj.Base

	fmt.Println("Base height:", base, "Prune height:", pruneHeight)

	if err := pruneKeys(
		blockStoreDB,
		"block",
		blockKeyInfos,
		func(store db.DB, ki PrefixAndSplitter) (uint64, error) {
			return deleteHeightRange(store, ki.prefix, 0, pruneHeight, ki.splitter)
		},
	); err != nil {
		return err
	}
	count, err := deleteAllByPrefix(blockStoreDB, []byte("BH:"))
	if err != nil {
		return fmt.Errorf("prune block BH: %w", err)
	}
	logger.Info("Pruned", "store", "block", "key", "BH:", "count", count)

	bsj.Base = int64(pruneHeight) + 1
	fmt.Println("Base height:", base, "Prune height:", pruneHeight)
	bytes, err := proto.Marshal(&bsj)
	if err != nil {
		panic(fmt.Sprintf("Could not marshal state bytes: %v", err))
	}
	if err := blockStoreDB.SetSync([]byte("blockStore"), bytes); err != nil {
		panic(err)
	}
	return nil
}

func pruneStateStore(stateStoreDB db.DB, pruneHeight uint64) error {
	return pruneKeys(
		stateStoreDB, "state", stateKeyInfos,
		func(store db.DB, key string) (uint64, error) {
			return deleteHeightRange(store, key, 0, pruneHeight, asciiHeightParser)
		},
	)
}

func pruneAppStore(appStore db.DB, pruneHeight uint64) error {
	return pruneKeys(
		appStore, "application", appKeyInfos,
		func(store db.DB, key string) (uint64, error) {
			return deleteHeightRange(store, key, 0, pruneHeight-1, asciiHeightParser)
		},
	)
}

func pruneBlockAndStateStore(blockStoreDB, stateStoreDB, appStore db.DB, pruneHeight uint64) error {
	g, _ := errgroup.WithContext(context.Background())

	g.Go(func() error { return pruneBlockStore(blockStoreDB, pruneHeight) })
	g.Go(func() error { return pruneStateStore(stateStoreDB, pruneHeight) })
	g.Go(func() error { return pruneAppStore(appStore, pruneHeight) })

	return g.Wait()
}

func pruneKeys[T any](
	store db.DB,
	storeName string,
	keyInfo []T,
	deleteFn func(db.DB, T) (uint64, error),
) error {
	for _, k := range keyInfo {
		count, err := deleteFn(store, k)
		if err != nil {
			return fmt.Errorf("prune %s key %v: %w", storeName, k, err)
		}
		logger.Info("Pruned", "store", storeName, "key", k, "count", count)
	}
	return nil
}

func deleteAllByPrefix(db db.DB, key []byte) (uint64, error) {
	rangeEnd := make([]byte, len(key))
	copy(rangeEnd, key)
	rangeEnd[len(rangeEnd)-1]++
	iter, err := db.Iterator(key, rangeEnd)
	defer iter.Close()

	if err != nil {
		return 0, err
	}
	batch := db.NewBatch()
	defer batch.Close()
	count := uint64(0)

	for ; iter.Valid(); iter.Next() {
		k := iter.Key()
		err = batch.Delete(k)
		count++
		if err != nil {
			return 0, err
		}
	}
	if err = batch.Write(); err != nil {
		return 0, err
	}
	return count, nil
}

type heightParser func(string) (uint64, error)

func asciiHeightParserTwoParts(numberPart string) (uint64, error) {
	parts := strings.SplitN(numberPart, ":", 2)
	return strconv.ParseUint(string(parts[0]), 10, 64)
}

func asciiHeightParser(numberPart string) (uint64, error) {
	return strconv.ParseUint(string(numberPart), 10, 64)
}

// Deletes all keys in the range <key>:<start> to <key>:<end>
// where start and end are left-padded with zeroes to the amount of base-10 digits
// in "end".
// For example, with key="test:", start=0 and end=1000, the keys
// test:0, test:1, ..., test:9, test:10, ..., test:99, test:100, ..., test:999, test:1000 will be deleted
func deleteHeightRange(db db.DB, key string, startHeight, endHeight uint64, heightParser heightParser) (uint64, error) {
	// keys are blobs of bytes, we can't do integer comparison,
	// even if a key looks like C:12345
	// we need to pad the range to match the right amount of digits
	maxDigits := len(fmt.Sprintf("%d", endHeight))
	var pruned uint64 = 0

	logger.Debug("Pruning key", "key", key)
	prunedLastBatch := 0
	for digits := maxDigits; digits >= 1; digits-- {
		rangeStart := uint64(math.Max(float64(startHeight), float64(math.Pow10(digits-1))))
		rangeEnd := uint64(math.Min(float64(endHeight), float64(math.Pow10(digits))-1))

		if rangeStart > rangeEnd {
			continue
		}

		startKey := []byte(fmt.Sprintf("%s%0*d", key, digits, rangeStart))
		endKey := []byte(fmt.Sprintf("%s%0*d", key, digits, rangeEnd))

		iter, err := db.Iterator(startKey, endKey)
		if err != nil {
			return pruned, fmt.Errorf("error creating iterator for digit length %d: %w", digits, err)
		}
		logger.Debug("Pruning range", "Start", string(startKey), "end", string(endKey))

		batch := db.NewBatch()

		for ; iter.Valid(); iter.Next() {
			k := iter.Key()
			// The keys are of format <key><height>
			// but <height> is an ascii-encoded integer; so when we query by range
			// we _will_ get keys which are beyond the expected maximum.
			// Parse the height from the key, and skip deletion if outside of the range
			numberPart := k[len(key):]
			number, err := heightParser(string(numberPart))
			if err != nil {
				logger.Error("Failed to parse height", "key", string(k), "err", err)
				continue
			}
			if number > endHeight {
				continue
			}
			pruned++
			prunedLastBatch++
			if err := batch.Delete(k); err != nil {
				iter.Close()
				batch.Close()
				return pruned, fmt.Errorf("error deleting key %s: %w", string(k), err)
			}

		}

		if err = batch.Write(); err != nil {
			return pruned, err
		}
		if err = batch.Close(); err != nil {
			return pruned, err
		}

		if err := iter.Error(); err != nil {
			iter.Close()
			return pruned, fmt.Errorf("iterator error for digit length %d: %w", digits, err)
		}

		iter.Close()
		if prunedLastBatch == 0 {
			break
		}
		prunedLastBatch = 0
	}

	return pruned, nil
}

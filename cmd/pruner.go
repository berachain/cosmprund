package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"cosmossdk.io/log"
	"cosmossdk.io/store/metrics"
	"cosmossdk.io/store/types"
	dbm "github.com/cometbft/cometbft-db"
	"github.com/cometbft/cometbft/state"
	cmtstore "github.com/cometbft/cometbft/store"
	db "github.com/cosmos/cosmos-db"
	"github.com/rs/zerolog"
	"github.com/syndtr/goleveldb/leveldb/opt"

	"github.com/binaryholdings/cosmos-pruner/internal/rootmulti"
)

// load db
// load app store and prune
// if immutable tree is not deletable we should import and export current state

var logger log.Logger

func setConfig(cfg *log.Config) {
	cfg.Level = zerolog.InfoLevel
}
func PruneAppState(dataDir string) error {
	o := opt.Options{
		DisableSeeksCompaction: true,
	}

	appDB, err := db.NewGoLevelDBWithOpts("application", dataDir, &o)
	if err != nil {
		return err
	}

	logger = log.NewLogger(os.Stderr, setConfig)
	logger.Info("pruning application state")

	appStore := rootmulti.NewStore(appDB, logger, metrics.NewNoOpMetrics())
	ver := rootmulti.GetLatestVersion(appDB)

	storeNames := []string{}
	if ver != 0 {
		cInfo, err := appStore.GetCommitInfo(ver)
		if err != nil {
			return err
		}

		for _, storeInfo := range cInfo.StoreInfos {
			// we only want to prune the stores with actual data.
			// sometimes in-memory stores get leaked to disk without data.
			// if that happens, the store's computed hash is empty as well.
			if len(storeInfo.CommitId.Hash) > 0 {
				storeNames = append(storeNames, storeInfo.Name)
			} else {
				logger.Info("skipping", storeInfo.Name, "store due to empty hash")
			}
		}
	}

	keys := types.NewKVStoreKeys(storeNames...)
	for _, value := range keys {
		appStore.MountStoreWithDB(value, types.StoreTypeIAVL, nil)
	}

	err = appStore.LoadLatestVersion()
	if err != nil {
		return err
	}

	versions := appStore.GetAllVersions()
	if len(versions) > 0 {
		v64 := make([]int64, len(versions))
		for i := 0; i < len(versions); i++ {
			v64[i] = int64(versions[i])
		}

		// -1 in case we have exactly 1 block in the DB
		targetHeight := v64[int64(len(v64))-int64(keepVersions)] - 1
		logger.Info("Pruning up to", "targetHeight", targetHeight)

		appStore.PruneStores(targetHeight)
	}

	if gcApplication {
		if err := runGC(dataDir, "application", o, appDB); err != nil {
			return err
		}
		appDB, err = db.NewGoLevelDBWithOpts("application", dataDir, &o)
		if err != nil {
			logger.Error("failed to re-open application")
			return err
		}
	}
	logger.Info("compacting application state")
	return appDB.ForceCompact(nil, nil)
}

// Implement a "GC" pass by copying only live data to a new DB
// This function will CLOSE dbToGC.
// This should be generic over dbs so we can also use dbm.GoLevelDB, but blockstore doesn't really
// benefit from GC
func runGC(dataDir string, dbName string, o opt.Options, dbToGC *db.GoLevelDB) error {
	logger.Info("starting garbage collection pass")
	gcDB, err := db.NewGoLevelDBWithOpts(fmt.Sprintf("%s_gc", dbName), dataDir, &o)
	if err != nil {
		logger.Error("Failed to open new application db", "err", err)
		return err
	}

	// Copy only live data
	iter, err := dbToGC.Iterator(nil, nil)
	if err != nil {
		logger.Error("Failed to get original db iterator", "err", err)
		return err
	}
	batchSize := 10_000
	batch := gcDB.NewBatch()
	count := 0

	for ; iter.Valid(); iter.Next() {
		batch.Set(iter.Key(), iter.Value())
		count++

		if count >= batchSize {
			batch.Write()
			batch.Close()
			batch = gcDB.NewBatch()
			count = 0
		}
	}
	logger.Info("Finished GC, closing")

	if count > 0 {
		batch.Write()
		batch.Close()
	}
	iter.Close()

	dbToGC.Close()
	gcDB.Close()

	if count == 0 {
		logger.Info("gc complete, but empty")
		return nil
	}

	oldPath := filepath.Join(dataDir, fmt.Sprintf("%s.db", dbName))
	backupPath := filepath.Join(dataDir, fmt.Sprintf("%s_backup.db", dbName))
	newPath := filepath.Join(dataDir, fmt.Sprintf("%s_gc.db", dbName))
	os.RemoveAll(backupPath)

	// Swap directories
	if err := os.Rename(oldPath, backupPath); err != nil {
		logger.Error("Failed to backup original DB", "err", err)
		return err
	}

	if err := os.Rename(newPath, oldPath); err != nil {
		logger.Error("Failed to swap GC DB", "err", err)
		// Try to restore original
		os.Rename(backupPath, oldPath)
		return err
	}

	return nil
}

// PruneCmtData prunes the cometbft blocks and state based on the amount of blocks to keep
func PruneCmtData(dataDir string) error {

	o := opt.Options{
		DisableSeeksCompaction: true,
	}

	// Get BlockStore
	blockStoreDB, err := dbm.NewGoLevelDBWithOpts("blockstore", dataDir, &o)
	if err != nil {
		return err
	}
	blockStore := cmtstore.NewBlockStore(blockStoreDB)

	// Get StateStore
	stateDB, err := dbm.NewGoLevelDBWithOpts("state", dataDir, &o)
	if err != nil {
		return err
	}

	stateStore := state.NewStore(stateDB, state.StoreOptions{})

	base := blockStore.Base()

	pruneHeight := blockStore.Height() - int64(keepBlocks)

	state, err := stateStore.Load()
	if err != nil {
		return err
	}

	logger.Info("pruning block store", "blockstore height", blockStore.Height(), "pruning target", pruneHeight)
	_, evidencePoint, err := blockStore.PruneBlocks(pruneHeight, state)
	if err != nil {
		return err
	}

	if err := blockStoreDB.Compact(nil, nil); err != nil {
		return err
	}

	if pruneHeight == evidencePoint {
		logger.Info("state store already pruned")
	} else if pruneHeight < evidencePoint {
		return fmt.Errorf("Asked to prune to %d but the chain is at %d", pruneHeight, evidencePoint)
	} else {

		logger.Info("pruning state store")
		err = stateStore.PruneStates(base, pruneHeight, evidencePoint)
		if err != nil {
			return err
		}
	}

	logger.Info("compacting state store")
	if err := stateDB.Compact(nil, nil); err != nil {
		return err
	}

	return nil
}

package cmd

import (
	"fmt"
	"os"

	"cosmossdk.io/log"
	"cosmossdk.io/store/metrics"
	"cosmossdk.io/store/types"
	evidencetypes "cosmossdk.io/x/evidence/types"
	feegrant "cosmossdk.io/x/feegrant"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	dbm "github.com/cometbft/cometbft-db"
	"github.com/cometbft/cometbft/state"
	cmtstore "github.com/cometbft/cometbft/store"
	db "github.com/cosmos/cosmos-db"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	authzkeeper "github.com/cosmos/cosmos-sdk/x/authz/keeper"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	consensusparamtypes "github.com/cosmos/cosmos-sdk/x/consensus/types"
	crisistypes "github.com/cosmos/cosmos-sdk/x/crisis/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	paramstypes "github.com/cosmos/cosmos-sdk/x/params/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	capabilitytypes "github.com/cosmos/ibc-go/modules/capability/types"
	ibctransfertypes "github.com/cosmos/ibc-go/v8/modules/apps/transfer/types"
	ibcexported "github.com/cosmos/ibc-go/v8/modules/core/exported"
	"github.com/syndtr/goleveldb/leveldb/opt"

	"github.com/binaryholdings/cosmos-pruner/internal/rootmulti"
)

// load db
// load app store and prune
// if immutable tree is not deletable we should import and export current state

func PruneAppState(dataDir string) error {

	o := opt.Options{
		DisableSeeksCompaction: true,
	}

	// Get BlockStore
	appDB, err := db.NewGoLevelDBWithOpts("application", dataDir, &o)
	if err != nil {
		return err
	}

	//TODO: need to get all versions in the store, setting randomly is too slow
	fmt.Println("pruning application state")

	// only mount keys from core sdk
	// todo allow for other keys to be mounted
	keys := types.NewKVStoreKeys(
		authtypes.StoreKey, banktypes.StoreKey, stakingtypes.StoreKey, crisistypes.StoreKey,
		minttypes.StoreKey, distrtypes.StoreKey, slashingtypes.StoreKey,
		govtypes.StoreKey, paramstypes.StoreKey, consensusparamtypes.StoreKey,
		ibcexported.StoreKey, upgradetypes.StoreKey, feegrant.StoreKey,
		evidencetypes.StoreKey, ibctransfertypes.StoreKey, capabilitytypes.StoreKey,
		authzkeeper.StoreKey,
	)

	keysToAdd := GetStoreKeysToAdd(app)
	if keysToAdd != nil {
		for key, value := range keysToAdd {
			keys[key] = value
		}
	}

	keysToDelete := GetStoreKeysToDelete(app)
	if keysToDelete != nil {
		for _, key := range keysToDelete {
			delete(keys, key)
		}
	}

	// TODO: cleanup app state
	appStore := rootmulti.NewStore(appDB, log.NewLogger(os.Stderr), metrics.NewNoOpMetrics())

	for _, value := range keys {
		appStore.MountStoreWithDB(value, types.StoreTypeIAVL, nil)
	}

	err = appStore.LoadLatestVersion()
	if err != nil {
		return err
	}

	versions := appStore.GetAllVersions()

	v64 := make([]int64, len(versions))
	for i := 0; i < len(versions); i++ {
		v64[i] = int64(versions[i])
	}

	fmt.Println(len(v64))

	appStore.PruneStores(int64(len(v64)) - int64(keepVersions))

	fmt.Println("compacting application state")
	if err := appDB.ForceCompact(nil, nil); err != nil {
		return err
	}

	//create a new app store
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

	fmt.Println("pruning block store")
	// prune block store
	_, evidencePoint, err := blockStore.PruneBlocks(pruneHeight, state)
	if err != nil {
		return err
	}

	fmt.Println("compacting block store")
	if err := blockStoreDB.Compact(nil, nil); err != nil {
		return err
	}

	fmt.Println("pruning state store")
	// prune state store
	err = stateStore.PruneStates(base, pruneHeight, evidencePoint)
	if err != nil {
		return err
	}

	fmt.Println("compacting state store")
	if err := stateDB.Compact(nil, nil); err != nil {
		return err
	}

	return nil
}

func GetStoreKeysToAdd(app string) map[string]*types.KVStoreKey {
	if app == "osmosis" {
		return types.NewKVStoreKeys(
			"icahost",        //icahosttypes.StoreKey,
			"gamm",           // gammtypes.StoreKey,
			"lockup",         //lockuptypes.StoreKey,
			"incentives",     // incentivestypes.StoreKey,
			"epochs",         // epochstypes.StoreKey,
			"poolincentives", //poolincentivestypes.StoreKey,
			"txfees",         // txfeestypes.StoreKey,
			"superfluid",     // superfluidtypes.StoreKey,
			"wasm",           // wasm.StoreKey,
			"tokenfactory",   //tokenfactorytypes.StoreKey,
		)
	} else if app == "cosmoshub" {
		return types.NewKVStoreKeys(
			"icahost", // icahosttypes.StoreKey
		)
	} else if app == "terra" { // terra classic
		return types.NewKVStoreKeys(
			"oracle",   // oracletypes.StoreKey,
			"market",   // markettypes.StoreKey,
			"treasury", //treasurytypes.StoreKey,
			"wasm",     // wasmtypes.StoreKey,
		)
	} else if app == "kava" {
		return types.NewKVStoreKeys(
			"feemarket", //feemarkettypes.StoreKey,
			"kavadist",  //kavadisttypes.StoreKey,
			"auction",   //auctiontypes.StoreKey,
			"issuance",  //issuancetypes.StoreKey,
			"bep3",      //bep3types.StoreKey,
			//"pricefeed", //pricefeedtypes.StoreKey,
			//"swap",      //swaptypes.StoreKey,
			"cdp",       //cdptypes.StoreKey,
			"hard",      //hardtypes.StoreKey,
			"committee", //committeetypes.StoreKey,
			"incentive", //incentivetypes.StoreKey,
			"evmutil",   //evmutiltypes.StoreKey,
			"savings",   //savingstypes.StoreKey,
			"bridge",    //bridgetypes.StoreKey,
		)
	} else if app == "evmos" {
		return types.NewKVStoreKeys(
			"evm",        // evmtypes.StoreKey,
			"feemarket",  // feemarkettypes.StoreKey,
			"inflation",  // inflationtypes.StoreKey,
			"erc20",      // erc20types.StoreKey,
			"incentives", // incentivestypes.StoreKey,
			"epochs",     // epochstypes.StoreKey,
			"claims",     // claimstypes.StoreKey,
			"vesting",    // vestingtypes.StoreKey,
		)
	} else if app == "gravitybridge" {
		return types.NewKVStoreKeys(
			"gravity",   //  gravitytypes.StoreKey,
			"bech32ibc", // bech32ibctypes.StoreKey,
		)
	} else if app == "sifchain" {
		return types.NewKVStoreKeys(
			"dispensation",  // disptypes.StoreKey,
			"ethbridge",     // ethbridgetypes.StoreKey,
			"clp",           // clptypes.StoreKey,
			"oracle",        // oracletypes.StoreKey,
			"tokenregistry", // tokenregistrytypes.StoreKey,
			"admin",         // admintypes.StoreKey,
		)
	} else if app == "starname" {
		return types.NewKVStoreKeys(
			"wasm",          // wasm.StoreKey,
			"configuration", // configuration.StoreKey,
			"starname",      // starname.DomainStoreKey,
		)
	} else if app == "regen" {
		return types.NewKVStoreKeys()
	} else if app == "akash" {
		return types.NewKVStoreKeys(
			"escrow",     // escrow.StoreKey,
			"deployment", // deployment.StoreKey,
			"market",     // market.StoreKey,
			"provider",   // provider.StoreKey,
			"audit",      // audit.StoreKey,
			"cert",       // cert.StoreKey,
			"inflation",  // inflation.StoreKey,
		)
	} else if app == "sentinel" {
		return types.NewKVStoreKeys(
			"distribution", // distributiontypes.StoreKey,
			"custommint",   // customminttypes.StoreKey,
			"swap",         // swaptypes.StoreKey,
			"vpn",          // vpntypes.StoreKey,
		)
	} else if app == "emoney" {
		return types.NewKVStoreKeys(
			"liquidityprovider", // lptypes.StoreKey,
			"issuer",            // issuer.StoreKey,
			"authority",         // authority.StoreKey,
			"market",            // market.StoreKey,
			//"market_indices",    // market.StoreKeyIdx,
			"buyback",   // buyback.StoreKey,
			"inflation", // inflation.StoreKey,
		)
	} else if app == "ixo" {
		return types.NewKVStoreKeys(
			"did",      // didtypes.StoreKey,
			"bonds",    // bondstypes.StoreKey,
			"payments", // paymentstypes.StoreKey,
			"project",  // projecttypes.StoreKey,
		)
	} else if app == "juno" {
		return types.NewKVStoreKeys(
			"icahost", // icahosttypes.StoreKey,
			"wasm",    // wasm.StoreKey,
		)
	} else if app == "likecoin" {
		return types.NewKVStoreKeys(
			// custom modules
			"iscn",    // iscntypes.StoreKey,
			"nft",     // nftkeeper.StoreKey,
			"likenft", // likenfttypes.StoreKey,
		)
	} else if app == "teritori" {
		// https://github.com/TERITORI/teritori-chain/blob/main/app/app.go#L323
		return types.NewKVStoreKeys(
			// common modules
			"packetfowardmiddleware", // routertypes.StoreKey,
			"icahost",                // icahosttypes.StoreKey,
			"wasm",                   // wasm.StoreKey,
			// custom modules
			"airdrop", // airdroptypes.StoreKey,
		)
	} else if app == "jackal" {
		// https://github.com/JackalLabs/canine-chain/blob/master/app/app.go#L347
		return types.NewKVStoreKeys(
			// common modules
			"wasm",    // wasm.StoreKey,
			"icahost", // icahosttypes.StoreKey,
			// custom modules
			"icacontroller", // icacontrollertypes.StoreKey, https://github.com/cosmos/ibc-go/blob/main/modules/apps/27-interchain-accounts/controller/types/keys.go#L5
			// intertx is a demo and not an officially supported IBC team implementation
			"intertx",       // intertxtypes.StoreKey, https://github.com/cosmos/interchain-accounts-demo/blob/8d4683081df0e1945be40be8ac18aa182106a660/x/inter-tx/types/keys.go#L4
			"rns",           // rnsmoduletypes.StoreKey, https://github.com/JackalLabs/canine-chain/blob/master/x/rns/types/keys.go#L5
			"storage",       // storagemoduletypes.StoreKey, https://github.com/JackalLabs/canine-chain/blob/master/x/storage/types/keys.go#L5
			"filetree",      // filetreemoduletypes.StoreKey, https://github.com/JackalLabs/canine-chain/blob/master/x/filetree/types/keys.go#L5
			"notifications", // notificationsmoduletypes.StoreKey, https://github.com/JackalLabs/canine-chain/blob/master/x/notifications/types/keys.go#L5
			"jklmint",       // jklmintmoduletypes.StoreKey, https://github.com/JackalLabs/canine-chain/blob/master/x/jklmint/types/keys.go#L7
			"oracle",        // https://github.com/JackalLabs/canine-chain/blob/master/x/oracle/types/keys.go#L5
		)
	} else if app == "kichain" {
		return types.NewKVStoreKeys(
			"wasm", // wasm.StoreKey,
		)
	} else if app == "cyber" {
		return types.NewKVStoreKeys(
			"liquidity", // liquiditytypes.StoreKey,
			"bandwidth", // bandwidthtypes.StoreKey,
			"graph",     // graphtypes.StoreKey,
			"rank",      // ranktypes.StoreKey,
			"grid",      // gridtypes.StoreKey,
			"dmn",       // dmntypes.StoreKey,
			"wasm",      // wasm.StoreKey,
		)
	} else if app == "cheqd" {
		return types.NewKVStoreKeys(
			"cheqd", // cheqdtypes.StoreKey,
		)
	} else if app == "stargaze" {
		return types.NewKVStoreKeys(
			"claim", // claimmoduletypes.StoreKey,
			"alloc", // allocmoduletypes.StoreKey,
			"wasm",  // wasm.StoreKey,
		)
	} else if app == "bandchain" {
		return types.NewKVStoreKeys(
			"oracle", // oracletypes.StoreKey,
		)
	} else if app == "chihuahua" {
		return types.NewKVStoreKeys(
			"wasm", // wasm.StoreKey,
		)
	} else if app == "bitcanna" {
		return types.NewKVStoreKeys(
			"bcna", // bcnamoduletypes.StoreKey,
		)
	} else if app == "konstellation" {
		return types.NewKVStoreKeys(
			"oracle", // racletypes.StoreKey,
			"wasm",   // wasm.StoreKey,
		)
	} else if app == "omniflixhub" {
		return types.NewKVStoreKeys(
			"alloc",       // alloctypes.StoreKey,
			"onft",        // onfttypes.StoreKey,
			"marketplace", // marketplacetypes.StoreKey,
		)
	} else if app == "vidulum" {
		return types.NewKVStoreKeys(
			"vidulum", // vidulummoduletypes.StoreKey,
		)
	} else if app == "beezee" {
		return types.NewKVStoreKeys(
			"scavenge", //scavengemodule.Storekey,
		)
	} else if app == "provenance" {
		return types.NewKVStoreKeys(
			"metadata",  // metadatatypes.StoreKey,
			"marker",    // markertypes.StoreKey,
			"attribute", // attributetypes.StoreKey,
			"name",      // nametypes.StoreKey,
			"msgfees",   // msgfeestypes.StoreKey,
			"wasm",      // wasm.StoreKey,
		)
	} else if app == "dig" {
		return types.NewKVStoreKeys(
			"wasm", // wasm.StoreKey,
		)
	} else if app == "comdex" {
		return types.NewKVStoreKeys(
			"wasm", // wasm.StoreKey,
		)
	} else if app == "bitsong" {
		return types.NewKVStoreKeys(
			"packetfowardmiddleware", // routertypes.StoreKey,
			"fantoken",               // fantokentypes.StoreKey,
			"merkledrop",             // merkledroptypes.StoreKey,
		)
	} else if app == "assetmantle" {
		return types.NewKVStoreKeys(
			"packetfowardmiddleware", // routerTypes.StoreKey,
			"icahost",                // icaHostTypes.StoreKey,
		)
	} else if app == "fetchhub" {
		return types.NewKVStoreKeys(
			"wasm", // wasm.StoreKey,
		)
	} else if app == "persistent" {
		return types.NewKVStoreKeys(
			"halving", // halving.StoreKey,
		)
	} else if app == "cryptoorgchain" {
		return types.NewKVStoreKeys(
			"chainmain", // chainmaintypes.StoreKey,
			"supply",    // supplytypes.StoreKey,
			"nft",       // nfttypes.StoreKey,
		)
	} else if app == "irisnet" {
		return types.NewKVStoreKeys(
			"guardian", // guardiantypes.StoreKey,
			"token",    // tokentypes.StoreKey,
			"nft",      // nfttypes.StoreKey,
			"htlc",     // htlctypes.StoreKey,
			"record",   // recordtypes.StoreKey,
			"coinswap", // coinswaptypes.StoreKey,
			"service",  // servicetypes.StoreKey,
			"oracle",   // oracletypes.StoreKey,
			"random",   // randomtypes.StoreKey,
			"farm",     // farmtypes.StoreKey,
			"tibc",     // tibchost.StoreKey,
			"NFT",      // tibcnfttypes.StoreKey,
			"MT",       // tibcmttypes.StoreKey,
			"mt",       // mttypes.StoreKey,
		)
	} else if app == "axelar" {
		return types.NewKVStoreKeys(
			"vote",       // voteTypes.StoreKey,
			"evm",        // evmTypes.StoreKey,
			"snapshot",   // snapTypes.StoreKey,
			"multisig",   // multisigTypes.StoreKey,
			"tss",        // tssTypes.StoreKey,
			"nexus",      // nexusTypes.StoreKey,
			"axelarnet",  // axelarnetTypes.StoreKey,
			"reward",     // rewardTypes.StoreKey,
			"permission", // permissionTypes.StoreKey,
			"wasm",       // wasm.StoreKey,
		)
	} else if app == "umee" {
		return types.NewKVStoreKeys(
			"gravity", // gravitytypes.StoreKey,
		)
	} else if app == "desmos" {
		// https://github.com/desmos-labs/desmos/blob/master/app/app.go#L255
		return types.NewKVStoreKeys(
			// common modules
			"wasm", // wasm.StoreKey,
			// IBC modules
			"icacontroller", // icacontrollertypes.StoreKey, https://github.com/cosmos/ibc-go/blob/main/modules/apps/27-interchain-accounts/controller/types/keys.go#L5
			"icahost",       // icahosttypes.StoreKey,
			// mainnet since v4.7.0
			"profiles",      // profilestypes.StoreKey,
			"relationships", // relationshipstypes.StoreKey,
			"subspaces",     // subspacestypes.StoreKey,
			"posts",         // poststypes.StoreKey,
			"reports",       // reports.StoreKey,
			"reactions",     // reactions.StoreKey,
			"fees",          // fees.StoreKey,
			// mainnet since v6.0
			"supply",
			"tokenfactory",
		)
	} else if app == "initia" {
		return types.NewKVStoreKeys(
			"mstaking",
		)
	}

	return nil
}

func GetStoreKeysToDelete(app string) []string {
	if app == "agoric" {
		return []string{
			"consensus",
			"crisis",
		}
	} else if app == "celestia" {
		return []string{
			"consensus",
			"crisis",
		}
	} else if app == "dymension" {
		return []string{
			"consensus",
			"crisis",
		}
	} else if app == "humans" {
		return []string{
			"consensus",
			"crisis",
		}
	} else if app == "kava" {
		return []string{
			"mint", // minttypes.StoreKey
		}
	} else if app == "initia" {
		return []string {
			"staking",
			"params",
			"mint",
		}
	} else if app == "jackal" {
		return []string {
			"consensus",
			"crisis",
		}
	} else if app == "lava" {
		return []string{
			"mint",
		}
	} else if app == "nillion" {
		return []string {
			"crisis",
		}
	} else if app == "osmosis" {
		return []string {
			"feegrant",
		}
	} else if app == "quicksilver" {
		return []string {
			"consensus",
			"crisis",
		}
	} else if app == "seda" {
		return []string {
			"params",
		}
	} else if app == "source" {
		return []string {
			"consensus",
			"crisis",
		}
	} else if app == "stride" {
		return []string {
			"authz",
		}
	}

	return nil
}

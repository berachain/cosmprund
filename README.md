# Cosmos-Pruner

This project prunes data from 'old' blocks from a CometBFT/Tendermint/Cosmos-sdk node.

Only the `block`, `state` and `application` databases are pruned.

Only `leveldb` and `pebbledb` backends are supported.


## How to use

Cosmprund works of a data directory that has the same structure of a normal cosmos-sdk/cometbft node. By default it will prune all but 10 blocks from cometbft, and all but 10 versions of application state.

```
# clone & build cosmprund repo
git clone https://github.com/ChorusOne/cosmprund/
cd cosmprund
make build

# stop your daemon/cosmovisor
sudo systemctl stop cosmovisor

# run cosmprund 
./build/cosmprund prune ~/.gaiad/data
```

Flags: 

- `blocks`: amount of blocks to keep on the node (Default 100)
- `versions`: amount of app state versions to keep on the node (Default 10)
- `cosmos-sdk`: If pruning a non cosmos-sdk chain, like Nomic, you only want to use cometbft pruning or if you want to only prune cometbft block & state as this is generally large on machines(Default true)
- `cometbft`: If the user wants to only prune application data they can disable pruning of cometbft data. (Default true)

## Metadata

You can also use `cosmprund db-info your-data-dir` and you will get a JSON back in this form:

```
{"chain_id":"allora-testnet-1","initial_height":1,"last_block_height":3039285,"app_hash":"1CA3A44FC14A6D08137245F5FCB32275DD1150FEE76E3AD7F31FC5B388474854","last_block_time":"2025-03-17T17:06:31.620566697Z"}
```

which you can use to get a correct `last_block_height` for your snapshots

## On speed

If you data directory is large, it will take a while to prune.

On our tests, a Berachain node with 150GB of data is pruned to ~50MB in ~10 minutes.


## On size

Depending on the chain, results will vary; for many chains we see a (compressed) snapshot in the 10~300MB range.

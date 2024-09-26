# Cosmos-Pruner

The goal of this project is to be able to prune a cometbft data base of blocks and an Cosmos-sdk application DB of all but the last X versions. This will allow people to not have to state sync every x days.

This tool works with a subset of modules.

## WARNING

Due to inefficiencies of iavl and the simple approach of this tool, it can take ages to prune the data of a large node.  

We are working on integrating this natively into the Cosmos-sdk and CometBFT

## How to use

Cosmprund works of a data directory that has the same structure of a normal cosmos-sdk/cometbft node. By default it will prune all but 10 blocks from cometbft, and all but 10 versions of application state.

> Note: Application pruning can take a very long time dependent on the size of the db. 


```
# clone & build cosmprund repo
git clone https://github.com/binaryholdings/cosmprund
cd cosmprund
make build

# stop daemon/cosmovisor
sudo systemctl stop cosmovisor

# run cosmprund 
./build/cosmprund prune ~/.gaiad/data --cosmos-sdk=false
```

Flags: 

- `blocks`: amount of blocks to keep on the node (Default 10)
- `versions`: amount of app state versions to keep on the node (Default 10)
- `cosmos-sdk`: If pruning a non cosmos-sdk chain, like Nomic, you only want to use cometbft pruning or if you want to only prune cometbft block & state as this is generally large on machines(Default true)
- `cometbft`: If the user wants to only prune application data they can disable pruning of cometbft data. (Default true)

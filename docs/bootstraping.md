# Bootstrapping in Alphabill - DRAFT
Alphabill uses a P2P network for communication between nodes. When a new node joins the network, it needs to connect to nodes that are already on the network to discover new peers. These nodes are called bootnodes. A list of bootnodes is provided via the `--bootnodes` configuration flag at node startup.

## Bootnode
There are no dedicated bootnodes; each Alphabill node can be used as a bootnode.

## Node Startup
At node startup, a list of bootnodes is provided via the `--bootnodes` configuration flag. The node will attempt to connect to all the provided bootnodes to speed up peer discovery. At least one of the connections must succeed for the node to proceed with startup. Otherwise, the node will exit with an error.

A node started without the `--bootnodes` configuration flag will not try to connect to other bootnodes. This node relies on the P2P network for peer discovery and should be used as a bootnode for other nodes.

### Example Command
```sh
./alphabill root-node run --bootnodes "/ip4/127.0.0.1/tcp/26662/p2p/<node-id>,/ip4/127.0.0.1/tcp/26663/p2p/<node-id>" ...
```

## Limitations
* Node startup order is important. A node chosen as a bootnode must be up and running before other nodes are started.
* A bootnode is not aware of other bootnodes.

## Troubleshooting
If a node fails to connect to any bootnodes:
* Verify that the bootnode addresses are correct and reachable.
* Check network connectivity between the node and the bootnodes.
* Ensure the bootnodes are running and accepting connections.
* Restart the node with updated configurations or alternative bootnodes.
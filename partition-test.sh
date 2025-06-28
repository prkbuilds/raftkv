#!/bin/bash

echo "Waiting for cluster to start..."
sleep 3

echo "Cluster status:"
docker ps --format "table {{.Names}}\t{{.Status}}" | grep raftfs

# Resolve container and network name dynamically
PARTITION_NODE=$(docker ps --format '{{.Names}}' | grep node2)
NETWORK_NAME=$(docker inspect "$PARTITION_NODE" --format '{{range $k, $v := .NetworkSettings.Networks}}{{$k}}{{end}}')

if [ -z "$PARTITION_NODE" ] || [ -z "$NETWORK_NAME" ]; then
    echo "Failed to resolve container or network name."
    exit 1
fi

echo
echo "Simulating network partition: disconnecting $PARTITION_NODE from $NETWORK_NAME"
docker network disconnect "$NETWORK_NAME" "$PARTITION_NODE"

sleep 20

echo
echo "Reconnecting $PARTITION_NODE to $NETWORK_NAME"
docker network connect "$NETWORK_NAME" "$PARTITION_NODE"


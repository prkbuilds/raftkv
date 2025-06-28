# RaftKV: Distributed Key-Value Store with Raft Consensus

![RaftKV Testing](./docs/testing_screenshot.png)

## Overview

RaftKV is a fault-tolerant distributed key-value store built using the [Raft consensus algorithm](https://raft.github.io/). It ensures **strong consistency** and **high availability** even in the presence of network partitions, node failures, and leader elections.

This project demonstrates a full Raft implementation in Go with gRPC communication, supporting:

- Leader election
- Log replication with consistency checks
- Client-facing API for `set` and `get` commands
- Network partition tolerance via Docker-based testing
- Simulated node failures and recovery scenarios

---

## Features

- **Consensus with Raft**: Guarantees consistency and reliability of replicated logs across nodes.
- **gRPC Communication**: Efficient and robust RPC framework between Raft nodes and clients.
- **Fault Tolerance**: Handles leader crashes, network partitions, and recovers automatically.
- **Key-Value Store**: Supports basic `set <key>=<value>` and `get <key>` operations.
- **Testing Suite**: Docker-based network partition simulation ensures >99.9% uptime under failure conditions.
- **Extensible Design**: Modular Go code with clean separation of consensus, RPC, and application logic.

---

## Client CLI / Tests

- Each Raft node runs a gRPC server that replicates logs and handles RPCs.
- Clients connect to any node; commands are forwarded to the leader.
- Nodes use heartbeat RPCs to maintain leadership and consistency.

---

## Getting Started

### Prerequisites

- Go 1.24+
- Docker & Docker Compose
- Protobuf compiler (`protoc`) with Go plugin

### Build & Run

```bash
# Build server binary
cd server
go build -o raft-server main.go

# Build client binary
cd ../client
go build -o raft-client main.go

# Start cluster with Docker Compose
docker-compose up --build
```

### Usage

Start a client session to interact with the RaftKV store:

```bash
./raft-client --mode=interactive
```

example commands:

```
> set mykey=hello
> get mykey
hello
```

---

## Testing & Network Partition Simulation

Network partitions and node failures are simulated using Docker commands to disconnect and reconnect nodes from the cluster network, verifying fault tolerance.

Example test script usage:

```bash
./partition-test.sh
```

This script:
- Waits for the cluster to start
- Disconnects a node’s network to simulate partition
- Reconnects the node after a delay
- Measures cluster uptime and availability during the partition

---

## Performance and Reliability

- Achieved >99.9% uptime during simulated node failures and network partitions.
- Leader election latency under 200ms.
- Log replication latency under 50ms in a local network environment.

---

## Project Structure

```bash
.
├── server          # Raft server node implementation
├── client          # CLI client for interacting with the cluster
├── proto           # Protobuf definitions and generated code
├── docs            # Documentation and screenshots
├── partition-test.sh # Network partition simulation script
├── docker-compose.yml
└── README.md
```

---

## Contributing

Contributions are welcome! Please open issues or pull requests for improvements, bug fixes, or new features.


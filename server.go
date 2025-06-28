package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"

	pb "github.com/prkbuilds/raft-kv/proto/raftpb"
	"github.com/prkbuilds/raft-kv/raft"
	"google.golang.org/grpc"
)

type RaftService struct {
	pb.UnimplementedRaftServer
	raftNode *raft.RaftNode
}

func (rs *RaftService) StartCommand(ctx context.Context, req *pb.StartCommandRequest) (*pb.StartCommandResponse, error) {
	index, term, isLeader := rs.raftNode.Start(req.Command)
	log.Printf("Received StartCommand RPC with command: %q", req.Command)
	return &pb.StartCommandResponse{
		Index:    int32(index),
		Term:     int32(term),
		IsLeader: isLeader,
	}, nil
}

func main() {
	id := flag.Int("id", 0, "Node ID (index into peers list)")
	port := flag.Int("port", 50051, "Port to listen on")
	flag.Parse()

	peers := []string{
		"localhost:50051",
		"localhost:50052",
	}

	if *id < 0 || *id >= len(peers) {
		log.Fatalf("Invalid node ID %d. Must be in range 0 to %d", *id, len(peers)-1)
	}

	applyCh := make(chan raft.ApplyMsg)
	node := raft.NewRaftNode(*id, peers, applyCh)

	grpcServer := grpc.NewServer()
	pb.RegisterRaftServer(grpcServer, &RaftService{raftNode: node})

	address := fmt.Sprintf(":%d", *port)
	lis, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatal("failed to listen: ", err)
	}

	log.Printf("Raft node %d listening on %s", *id, address)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatal("failed to serve: ", err)
	}
}

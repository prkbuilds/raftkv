package main

import (
	"context"
	"fmt"
	"log"
	"time"

	pb "github.com/prkbuilds/raft-kv/proto/raftpb"
	"google.golang.org/grpc"
)

func main() {
	// List of Raft server addresses (same as your Docker Compose ports)
	servers := []string{
		"localhost:50051",
		"localhost:50052",
		"localhost:50053",
	}

	var client pb.RaftClient
	var conn *grpc.ClientConn
	var err error

	// Try to find the leader by sending StartCommand RPC until one accepts (isLeader == true)
	for _, addr := range servers {
		conn, err = grpc.Dial(addr, grpc.WithInsecure())
		if err != nil {
			log.Printf("Failed to connect to %s: %v", addr, err)
			continue
		}
		defer conn.Close()

		client = pb.NewRaftClient(conn)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		req := &pb.StartCommandRequest{
			Command: "set x=42",
		}

		resp, err := client.StartCommand(ctx, req)
		if err != nil {
			log.Printf("Error calling StartCommand on %s: %v", addr, err)
			continue
		}

		if resp.IsLeader {
			fmt.Printf("Leader is at %s, command appended at index %d term %d\n", addr, resp.Index, resp.Term)
			return
		} else {
			log.Printf("Node %s is not leader", addr)
		}
	}

	log.Println("No leader found or no node accepted the command")
}

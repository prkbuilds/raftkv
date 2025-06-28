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
	"google.golang.org/grpc/reflection"
)

type RaftService struct {
	pb.UnimplementedRaftServer
	raftNode *raft.RaftNode
}

func (rs *RaftService) Set(ctx context.Context, req *pb.SetRequest) (*pb.SetResponse, error) {
	index, term, isLeader := rs.raftNode.Set(req.Command)
	return &pb.SetResponse{
		Index:    int32(index),
		Term:     int32(term),
		IsLeader: isLeader,
	}, nil
}

func (rs *RaftService) Get(ctx context.Context, req *pb.GetRequest) (*pb.GetResponse, error) {
	val, found := rs.raftNode.Get(req.Key)
	return &pb.GetResponse{
		Value: val,
		Found: found,
	}, nil
}

func (rs *RaftService) RequestVote(ctx context.Context, req *pb.RequestVoteArgs) (*pb.RequestVoteReply, error) {
	args := &raft.RequestVoteArgs{
		Term:         int(req.Term),
		CandidateId:  int(req.CandidateId),
		LastLogIndex: int(req.LastLogIndex),
		LastLogTerm:  int(req.LastLogTerm),
	}
	reply := &raft.RequestVoteReply{}
	rs.raftNode.HandleRequestVote(args, reply)
	return &pb.RequestVoteReply{
		Term:        int32(reply.Term),
		VoteGranted: reply.VoteGranted,
	}, nil
}

func (rs *RaftService) AppendEntries(ctx context.Context, req *pb.AppendEntriesArgs) (*pb.AppendEntriesReply, error) {
	entries := make([]raft.LogEntry, len(req.Entries))
	for i, e := range req.Entries {
		entries[i] = raft.LogEntry{
			Term:    int(e.Term),
			Command: e.Command,
		}
	}

	args := &raft.AppendEntriesArgs{
		Term:         int(req.Term),
		LeaderId:     int(req.LeaderId),
		PrevLogIndex: int(req.PrevLogIndex),
		PrevLogTerm:  int(req.PrevLogTerm),
		Entries:      entries,
		LeaderCommit: int(req.LeaderCommit),
	}
	reply := &raft.AppendEntriesReply{}
	rs.raftNode.HandleAppendEntries(args, reply)
	return &pb.AppendEntriesReply{
		Term:    int32(reply.Term),
		Success: reply.Success,
	}, nil
}

func main() {
	id := flag.Int("id", 0, "Node ID (index into peers list)")
	port := flag.Int("port", 50051, "Port to listen on")
	flag.Parse()

	peers := []string{
		"node0:50051",
		"node1:50052",
		"node2:50053",
	}

	if *id < 0 || *id >= len(peers) {
		log.Fatalf("Invalid node ID %d. Must be in range 0 to %d", *id, len(peers)-1)
	}

	applyCh := make(chan raft.ApplyMsg, 100)
	node := raft.NewRaftNode(*id, peers, applyCh)

	go func() {
		for msg := range applyCh {
			if msg.CommandValid {
				// log.Printf("Node %d: applied command at index %d: %v", *id, msg.CommandIndex, msg.Command)
			}
		}
	}()

	grpcServer := grpc.NewServer()
	reflection.Register(grpcServer)
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

package raft

import (
	"log"
	"sync"
	"time"

	pb "github.com/prkbuilds/raft-kv/proto/raftpb"
	"google.golang.org/grpc"
)

type RaftNode struct {
	mu sync.Mutex

	id          int
	peers       []string
	clients     []pb.RaftClient
	state       State
	currentTerm int
	votedFor    *int
	log         []LogEntry

	commitIndex int
	lastApplied int

	nextIndex  []int
	matchIndex []int

	electionTimer *time.Timer
	heartbeatCh   chan bool
	applyCh       chan ApplyMsg

	kvStore map[string]string
}

func NewRaftNode(id int, peers []string, applyCh chan ApplyMsg) *RaftNode {
	clients := make([]pb.RaftClient, 0)
	for _, addr := range peers {
		conn, err := grpc.Dial(addr, grpc.WithInsecure())
		if err != nil {
			log.Fatalf("Failed to connect to %s: %v", addr, err)
		}
		clients = append(clients, pb.NewRaftClient(conn))
	}

	rn := &RaftNode{
		id:          id,
		peers:       peers,
		clients:     clients,
		state:       Follower,
		votedFor:    nil,
		log:         make([]LogEntry, 0),
		commitIndex: -1,
		lastApplied: -1,
		heartbeatCh: make(chan bool, 1),
		applyCh:     applyCh,
		kvStore:     make(map[string]string),
	}

	rn.resetElectionTimer()
	go rn.runElectionTimer()
	go rn.runApplier()
	return rn
}

func (rn *RaftNode) peerIndex(peer string) int {
	for i, p := range rn.peers {
		if p == peer {
			return i
		}
	}
	return -1
}

func (rn *RaftNode) lastLogIndex() int {
	return len(rn.log) - 1
}

func (rn *RaftNode) lastLogTerm() int {
	if len(rn.log) == 0 {
		return 0
	}
	return rn.log[len(rn.log)-1].Term
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

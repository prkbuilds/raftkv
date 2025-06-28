package raft

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sort"
	"sync"
	"time"

	pb "github.com/prkbuilds/raft-kv/proto/raftpb"
	"google.golang.org/grpc"
)

type State string

const (
	Follower  State = "Follower"
	Candidate State = "Candidate"
	Leader    State = "Leader"
)

type LogEntry struct {
	Term    int
	Command interface{}
}

type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int
}

type RequestVoteArgs struct {
	Term         int
	CandidateId  int
	LastLogIndex int
	LastLogTerm  int
}

type RequestVoteReply struct {
	Term        int
	VoteGranted bool
}

type AppendEntriesArgs struct {
	Term         int
	LeaderId     int
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []LogEntry
	LeaderCommit int
}

type AppendEntriesReply struct {
	Term    int
	Success bool
}

type RaftNode struct {
	mu          sync.Mutex
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
		log:         make([]LogEntry, 0),
		votedFor:    nil,
		applyCh:     applyCh,
		heartbeatCh: make(chan bool, 1),
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

func (rn *RaftNode) Start(command interface{}) (int, int, bool) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	if rn.state != Leader {
		log.Printf("Rejecting command, node %d is not leader", rn.id)
		return -1, rn.currentTerm, false
	}

	entry := LogEntry{Term: rn.currentTerm, Command: command}
	rn.log = append(rn.log, entry)
	index := len(rn.log) - 1
	term := rn.currentTerm

	log.Printf("Leader %d appended command at index %d, term %d", rn.id, index, term)
	rn.replicateLog()

	return index, term, true
}

func (rn *RaftNode) resetElectionTimer() {
	if rn.electionTimer != nil {
		rn.electionTimer.Stop()
	}
	timeout := time.Duration(300+rand.Intn(200)) * time.Millisecond
	rn.electionTimer = time.NewTimer(timeout)
}

func (rn *RaftNode) runElectionTimer() {
	for {
		rn.mu.Lock()
		state := rn.state
		rn.mu.Unlock()

		select {
		case <-rn.electionTimer.C:
			if state != Leader {
				go rn.startElection()
			}
		case <-rn.heartbeatCh:
			rn.resetElectionTimer()
		}
	}
}

func (rn *RaftNode) runApplier() {
	for {
		time.Sleep(10 * time.Millisecond)
		rn.mu.Lock()
		for rn.lastApplied < rn.commitIndex {
			rn.lastApplied++
			msg := ApplyMsg{
				CommandValid: true,
				Command:      rn.log[rn.lastApplied].Command,
				CommandIndex: rn.lastApplied,
			}
			rn.mu.Unlock()
			rn.applyCh <- msg
			rn.mu.Lock()
		}
		rn.mu.Unlock()
	}
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

func (rn *RaftNode) startElection() {
	rn.mu.Lock()
	rn.state = Candidate
	rn.currentTerm++
	rn.votedFor = &rn.id
	currentTerm := rn.currentTerm
	lastLogIndex := rn.lastLogIndex()
	lastLogTerm := rn.lastLogTerm()
	rn.resetElectionTimer()
	rn.mu.Unlock()

	log.Printf("Node %d started election in term %d", rn.id, currentTerm)

	votes := 1
	votesCh := make(chan bool, len(rn.peers)-1)

	for i := range rn.peers {
		if i == rn.id {
			continue
		}

		go func(i int) {
			args := RequestVoteArgs{
				Term:         currentTerm,
				CandidateId:  rn.id,
				LastLogIndex: lastLogIndex,
				LastLogTerm:  lastLogTerm,
			}
			var reply RequestVoteReply
			ok := rn.sendRequestVote(rn.peers[i], &args, &reply)
			votesCh <- ok && reply.VoteGranted
		}(i)
	}

	for i := 0; i < len(rn.peers)-1; i++ {
		if <-votesCh {
			votes++
		}
	}

	rn.mu.Lock()
	defer rn.mu.Unlock()
	if rn.state == Candidate && votes > len(rn.peers)/2 {
		rn.becomeLeader()
	}
}

func (rn *RaftNode) becomeLeader() {
	rn.state = Leader
	rn.nextIndex = make([]int, len(rn.peers))
	rn.matchIndex = make([]int, len(rn.peers))
	for i := range rn.peers {
		rn.nextIndex[i] = rn.lastLogIndex() + 1
		rn.matchIndex[i] = 0
	}
	log.Printf("Node %d became Leader in term %d", rn.id, rn.currentTerm)
	rn.replicateLog()
}

func (rn *RaftNode) replicateLog() {
	for i, peer := range rn.peers {
		if i == rn.id {
			continue
		}

		go func(i int, peer string) {
			rn.mu.Lock()
			prevLogIndex := rn.nextIndex[i] - 1
			prevLogTerm := 0
			if prevLogIndex >= 0 && prevLogIndex < len(rn.log) {
				prevLogTerm = rn.log[prevLogIndex].Term
			}
			entries := make([]LogEntry, len(rn.log[rn.nextIndex[i]:]))
			copy(entries, rn.log[rn.nextIndex[i]:])
			args := AppendEntriesArgs{
				Term:         rn.currentTerm,
				LeaderId:     rn.id,
				PrevLogIndex: prevLogIndex,
				PrevLogTerm:  prevLogTerm,
				Entries:      entries,
				LeaderCommit: rn.commitIndex,
			}
			rn.mu.Unlock()

			var reply AppendEntriesReply
			ok := rn.sendAppendEntries(peer, &args, &reply)
			if ok {
				rn.mu.Lock()
				defer rn.mu.Unlock()
				if reply.Success {
					rn.matchIndex[i] = args.PrevLogIndex + len(args.Entries)
					rn.nextIndex[i] = rn.matchIndex[i] + 1
					rn.updateCommitIndex()
				} else if reply.Term > rn.currentTerm {
					rn.currentTerm = reply.Term
					rn.state = Follower
					rn.votedFor = nil
					rn.resetElectionTimer()
				} else {
					rn.nextIndex[i]--
				}
			}
		}(i, peer)
	}
}

func (rn *RaftNode) updateCommitIndex() {
	matchIndexes := append([]int{}, rn.matchIndex...)
	matchIndexes[rn.id] = rn.lastLogIndex()
	sort.Ints(matchIndexes)
	N := matchIndexes[len(rn.peers)/2]
	if N > rn.commitIndex && rn.log[N].Term == rn.currentTerm {
		rn.commitIndex = N
	}
}

func (rn *RaftNode) sendAppendEntries(peer string, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	i := rn.peerIndex(peer)
	if i == -1 {
		return false
	}

	entries := []*pb.LogEntry{}
	for _, e := range args.Entries {
		entries = append(entries, &pb.LogEntry{
			Term:    int32(e.Term),
			Command: fmt.Sprintf("%v", e.Command),
		})
	}

	req := &pb.AppendEntriesArgs{
		Term:         int32(args.Term),
		LeaderId:     int32(args.LeaderId),
		PrevLogIndex: int32(args.PrevLogIndex),
		PrevLogTerm:  int32(args.PrevLogTerm),
		Entries:      entries,
		LeaderCommit: int32(args.LeaderCommit),
	}

	resp, err := rn.clients[i].AppendEntries(context.Background(), req)
	if err != nil {
		return false
	}

	reply.Term = int(resp.Term)
	reply.Success = resp.Success
	return true
}

func (rn *RaftNode) sendRequestVote(peer string, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	i := rn.peerIndex(peer)
	if i == -1 {
		return false
	}

	req := &pb.RequestVoteArgs{
		Term:         int32(args.Term),
		CandidateId:  int32(args.CandidateId),
		LastLogIndex: int32(args.LastLogIndex),
		LastLogTerm:  int32(args.LastLogTerm),
	}

	resp, err := rn.clients[i].RequestVote(context.Background(), req)
	if err != nil {
		return false
	}

	reply.Term = int(resp.Term)
	reply.VoteGranted = resp.VoteGranted
	return true
}

func (rn *RaftNode) HandleRequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	if args.Term < rn.currentTerm {
		reply.Term = rn.currentTerm
		reply.VoteGranted = false
		return
	}

	if args.Term > rn.currentTerm {
		rn.currentTerm = args.Term
		rn.state = Follower
		rn.votedFor = nil
	}

	upToDate := args.LastLogTerm > rn.lastLogTerm() ||
		(args.LastLogTerm == rn.lastLogTerm() && args.LastLogIndex >= rn.lastLogIndex())

	if (rn.votedFor == nil || *rn.votedFor == args.CandidateId) && upToDate {
		rn.votedFor = &args.CandidateId
		reply.VoteGranted = true
	} else {
		reply.VoteGranted = false
	}

	reply.Term = rn.currentTerm
	rn.resetElectionTimer()
}

func (rn *RaftNode) HandleAppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	if args.Term < rn.currentTerm {
		reply.Term = rn.currentTerm
		reply.Success = false
		return
	}

	select {
	case rn.heartbeatCh <- true:
	default:
	}

	if args.Term > rn.currentTerm {
		rn.currentTerm = args.Term
		rn.votedFor = nil
		rn.state = Follower
	}

	if args.PrevLogIndex >= len(rn.log) || (args.PrevLogIndex > 0 && rn.log[args.PrevLogIndex].Term != args.PrevLogTerm) {
		reply.Term = rn.currentTerm
		reply.Success = false
		return
	}

	index := args.PrevLogIndex + 1
	for i, entry := range args.Entries {
		if index < len(rn.log) {
			if rn.log[index].Term != entry.Term {
				rn.log = rn.log[:index]
				rn.log = append(rn.log, args.Entries[i:]...)
				break
			}
		} else {
			rn.log = append(rn.log, args.Entries[i:]...)
			break
		}
		index++
	}

	if args.LeaderCommit > rn.commitIndex {
		rn.commitIndex = min(args.LeaderCommit, len(rn.log)-1)
	}

	reply.Term = rn.currentTerm
	reply.Success = true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

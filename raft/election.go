package raft

import (
	"context"
	"log"
	"math/rand"
	"time"

	pb "github.com/prkbuilds/raft-kv/proto/raftpb"
)

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
		go rn.sendHeartbeats()
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

func (rn *RaftNode) resetElectionTimer() {
	if rn.electionTimer != nil {
		rn.electionTimer.Stop()
	}
	timeout := time.Duration(500+rand.Intn(200)) * time.Millisecond
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

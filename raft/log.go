package raft

import (
	"context"
	"fmt"
	"log"
	"slices"
	"sort"
	"time"

	pb "github.com/prkbuilds/raft-kv/proto/raftpb"
)

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

func (rn *RaftNode) sendAppendEntries(peer string, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	i := rn.peerIndex(peer)
	if i == -1 {
		return false
	}

	entries := []*pb.LogEntry{}
	for _, e := range args.Entries {
		log.Printf("Sending log entry: %v", e)
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
				log.Printf("Adding entries: %v", args.Entries[i:])
				rn.log = rn.log[:index]
				rn.log = append(rn.log, args.Entries[i:]...)
				break
			}
		} else {
			log.Printf("Adding entries: %v", args.Entries[i:])
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

func (rn *RaftNode) updateCommitIndex() {
	matchIndexes := slices.Clone(rn.matchIndex)
	matchIndexes[rn.id] = rn.lastLogIndex()
	sort.Ints(matchIndexes)
	N := matchIndexes[len(rn.peers)/2]

	if N > rn.commitIndex && rn.log[N].Term == rn.currentTerm {
		log.Printf("Leader %d: committing log at index %d (term %d)", rn.id, N, rn.currentTerm)
		rn.commitIndex = N

		for i := rn.lastApplied + 1; i <= rn.commitIndex; i++ {
			if ch, ok := rn.pendingCommits[i]; ok {
				ch <- true
				delete(rn.pendingCommits, i)
			}
		}
	}
}

func (rn *RaftNode) sendHeartbeats() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		rn.mu.Lock()
		if rn.state != Leader {
			rn.mu.Unlock()
			return
		}
		rn.mu.Unlock()

		rn.replicateLog()
		<-ticker.C
	}
}

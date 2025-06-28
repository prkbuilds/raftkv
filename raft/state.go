package raft

import (
	"log"
	"strings"
	"time"
)

func (rn *RaftNode) applyCommittedLogs() {
	for {
		rn.mu.Lock()
		for rn.lastApplied < rn.commitIndex {
			rn.lastApplied++
			index := rn.lastApplied
			rn.applyLog(index)
		}
		rn.mu.Unlock()
		time.Sleep(50 * time.Millisecond)
	}
}

func (rn *RaftNode) applyLog(index int) {
	if index >= len(rn.log) {
		return
	}
	entry := rn.log[index]
	cmdStr, ok := entry.Command.(string)
	if !ok {
		log.Printf("Node %d: applyLog: command at index %d is not a string", rn.id, index)
		return
	}
	parts := strings.SplitN(cmdStr, " ", 2)
	if len(parts) == 2 && parts[0] == "set" {
		kv := strings.SplitN(parts[1], "=", 2)
		if len(kv) == 2 {
			key := kv[0]
			value := kv[1]
			rn.kvStore[key] = value
			log.Printf("Node %d: applied command at index %d -> %s=%s", rn.id, index, key, value)
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
			log.Printf("Node %d: applied command at index %d -> %v", rn.id, msg.CommandIndex, msg.Command)
			rn.applyCh <- msg
			rn.mu.Lock()
		}
		rn.mu.Unlock()
	}
}

package raft

import (
	"log"
	"strings"
	"time"
)

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
		}
	}

	msg := ApplyMsg{
		CommandValid: true,
		Command:      entry.Command,
		CommandIndex: index,
	}

	select {
	case rn.applyCh <- msg:
	default:
		log.Printf("Node %d: applyCh is full, dropping apply message for index %d", rn.id, index)
	}
}

func (rn *RaftNode) runApplier() {
	for {
		time.Sleep(10 * time.Millisecond)
		rn.mu.Lock()
		for rn.lastApplied < rn.commitIndex {
			nextIndex := rn.lastApplied + 1
			rn.lastApplied = nextIndex
			rn.mu.Unlock()
			rn.applyLog(rn.lastApplied)
			rn.mu.Lock()
		}
		rn.mu.Unlock()
	}
}

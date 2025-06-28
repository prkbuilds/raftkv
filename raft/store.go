package raft

import "log"

func (rn *RaftNode) Set(command interface{}) (int, int, bool) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	if rn.state != Leader {
		log.Printf("Rejecting command, node %d is not leader", rn.id)
		return -1, rn.currentTerm, false
	}

	entry := LogEntry{Term: rn.currentTerm, Command: command}
	rn.log = append(rn.log, entry)
	index := len(rn.log) - 1

	return index, rn.currentTerm, true
}

func (rn *RaftNode) Get(key string) (string, bool) {
	rn.mu.Lock()
	defer rn.mu.Unlock()
	val, ok := rn.kvStore[key]
	return val, ok
}

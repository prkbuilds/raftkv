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
	term := rn.currentTerm

	log.Printf("Leader %d appended command at index %d, term %d", rn.id, index, term)

	ch := make(chan bool, 1)
	rn.pendingCommits[index] = ch

	go rn.replicateLog()

	return index, rn.currentTerm, true
}

func (rn *RaftNode) Get(key string) (string, bool) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	log.Printf("Node %d: Get called for key %s", rn.id, key)
	val, ok := rn.kvStore[key]
	log.Printf("fetched value: %s", val)
	return val, ok
}

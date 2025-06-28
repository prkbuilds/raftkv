package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	pb "github.com/prkbuilds/raft-kv/proto/raftpb"
	"google.golang.org/grpc"
)

var nodeAddrs = []string{":50051", ":50052", ":50053"}

func trySetCommand(addr, command string) bool {
	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		log.Printf("Failed to connect to %s: %v", addr, err)
		return false
	}
	defer conn.Close()

	client := pb.NewRaftClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req := &pb.SetRequest{Command: command}
	res, err := client.Set(ctx, req)
	if err != nil {
		log.Printf("RPC error from %s: %v", addr, err)
		return false
	}

	if res.IsLeader {
		fmt.Printf("Leader at %s accepted command. Index: %d, Term: %d\n", addr, res.Index, res.Term)
		return true
	}
	return false
}

func tryGetCommand(addr, key string) (string, bool) {
	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		log.Printf("Failed to connect to %s: %v", addr, err)
		return "", false
	}
	defer conn.Close()

	client := pb.NewRaftClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req := &pb.GetRequest{Key: key}
	res, err := client.Get(ctx, req)
	if err != nil {
		log.Printf("RPC error from %s: %v", addr, err)
		return "", false
	}

	if res.Found {
		return res.Value, true
	}
	return "(not found)", true
}

func runTest(duration int) {
	log.Printf("Starting Raft KV Test for %d seconds...\n", duration)
	total := 0
	success := 0
	verified := 0
	leaderAddr := ""
	start := time.Now()

	for i := 0; time.Since(start) < time.Duration(duration)*time.Second; i++ {
		key := fmt.Sprintf("key%d", i)
		value := fmt.Sprintf("value%d", i)
		cmd := fmt.Sprintf("set %s=%s", key, value)
		ok := false

		if leaderAddr != "" {
			ok = trySetCommand(leaderAddr, cmd)
			if !ok {
				leaderAddr = ""
			}
		}

		if leaderAddr == "" {
			for _, addr := range nodeAddrs {
				if trySetCommand(addr, cmd) {
					log.Printf("Leader at %s accepted command: %s", addr, cmd)
					leaderAddr = addr
					ok = true
					break
				}
			}
		}
		time.Sleep(200 * time.Millisecond)

		total++
		if ok {
			success++

			// Attempt to get the key and verify the value
			gotValue := ""
			for _, addr := range nodeAddrs {
				val, found := tryGetCommand(addr, key)
				if found {
					gotValue = val
					break
				}
			}

			if gotValue == value {
				verified++
			} else {
				log.Printf("Verification failed for %s: expected=%s, got=%s", key, value, gotValue)
			}
		} else {
			log.Printf("Failed to send command: %s", cmd)
		}

		uptime := float64(success) / float64(total) * 100
		accuracy := float64(verified) / float64(success) * 100
		log.Printf("[Test] Uptime: %.2f%% (%d/%d), Verified: %.2f%% (%d/%d)", uptime, success, total, accuracy, verified, success)

		time.Sleep(200 * time.Millisecond)
	}

	log.Printf("Test completed.\nTotal requests: %d\nSuccessful: %d\nVerified: %d\nUptime: %.2f%%\nCorrectness: %.2f%%",
		total, success, verified,
		float64(success)/float64(total)*100,
		float64(verified)/float64(success)*100)
}

func runInteractive() {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("Raft KV Client (Interactive)")
	fmt.Println("Commands:")
	fmt.Println("  set <key>=<value>")
	fmt.Println("  get <key>")
	fmt.Println("Type 'exit' to quit")

	for {
		fmt.Print("> ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "exit" {
			break
		}

		if strings.HasPrefix(input, "get ") {
			key := strings.TrimSpace(strings.TrimPrefix(input, "get "))
			sent := false
			for _, addr := range nodeAddrs {
				value, ok := tryGetCommand(addr, key)
				if ok {
					fmt.Printf("Response from %s: %s = %s\n", addr, key, value)
					sent = true
					break
				}
			}
			if !sent {
				fmt.Println("Failed to get key from any node.")
			}
			continue
		}

		sent := false
		for _, addr := range nodeAddrs {
			if trySetCommand(addr, input) {
				sent = true
				break
			}
		}
		if !sent {
			fmt.Println("Failed to send command to any leader. Try again.")
		}
	}
}

func main() {
	mode := flag.String("mode", "test", "Mode to run the program in. Options: interactive, test")
	testDuration := flag.Int("duration", 300, "Duration to run the test for in seconds. Only used when mode is test.")

	switch *mode {
	case "interactive":
		runInteractive()
	case "test":
		runTest(*testDuration)
	default:
		log.Fatalf("Unknown mode: %s", *mode)
	}
}

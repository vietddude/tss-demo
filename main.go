package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"tss-demo/node"
)

func main() {
	// Define configurations for 3 nodes
	configs := []node.NodeConfig{
		{
			ID:       1,
			GRPCPort: ":50051",
			HTTPPort: ":8081",
			PeerAddrs: map[uint16]string{
				2: "localhost:50052",
				3: "localhost:50053",
			},
		},
		{
			ID:       2,
			GRPCPort: ":50052",
			HTTPPort: ":8082",
			PeerAddrs: map[uint16]string{
				1: "localhost:50051",
				3: "localhost:50053",
			},
		},
		{
			ID:       3,
			GRPCPort: ":50053",
			HTTPPort: ":8083",
			PeerAddrs: map[uint16]string{
				1: "localhost:50051",
				2: "localhost:50052",
			},
		},
	}

	// Start all nodes
	for _, config := range configs {
		config := config // Create new variable for goroutine
		go node.StartNode(config)
	}

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down TSS nodes...")
}

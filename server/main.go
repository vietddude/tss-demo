package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tss-demo/tss"
)

// Logger implementation
type StdLogger struct{}

func (l *StdLogger) Debugf(format string, args ...interface{}) {
	log.Printf("[DEBUG] "+format, args...)
}

func (l *StdLogger) Warnf(format string, args ...interface{}) {
	log.Printf("[WARN] "+format, args...)
}

func (l *StdLogger) Errorf(format string, args ...interface{}) {
	log.Printf("[ERROR] "+format, args...)
}

// Config for a TSS node
type NodeConfig struct {
	ID        uint16
	GRPCPort  string
	HTTPPort  string
	PeerAddrs map[uint16]string // map of peer ID to their gRPC address
}

func startNode(config NodeConfig) {
	logger := &StdLogger{}

	// Create new TSS node with logger
	node := tss.NewTSSNode(config.ID, logger)

	// Start gRPC server
	go func() {
		logger.Debugf("Starting gRPC server for node %d on port %s", config.ID, config.GRPCPort)
		if err := node.StartServer(config.GRPCPort); err != nil {
			logger.Errorf("Failed to start gRPC server: %v", err)
			os.Exit(1)
		}
	}()

	// Give the gRPC server time to start
	time.Sleep(2 * time.Second)

	// Connect to peers with retry
	for peerID, addr := range config.PeerAddrs {
		peerID := peerID // Create new variable for goroutine
		addr := addr
		go func() {
			node.ConnectToPeerWithRetry(peerID, addr)
		}()
	}

	// Setup HTTP server with mux for each node
	mux := http.NewServeMux()
	mux.HandleFunc("/wallet/create", node.CreateWallet)
	mux.HandleFunc("/wallet/sign", node.Sign)

	// Start HTTP server with dedicated mux
	server := &http.Server{
		Addr:    config.HTTPPort,
		Handler: mux,
	}

	go func() {
		logger.Debugf("Starting HTTP server for node %d on port %s", config.ID, config.HTTPPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Errorf("Failed to start HTTP server: %v", err)
			os.Exit(1)
		}
	}()
}

func main() {
	// Define configurations for 3 nodes
	configs := []NodeConfig{
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
		go startNode(config)
	}

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down TSS nodes...")
}

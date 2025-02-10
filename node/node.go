package node

import (
	"net/http"
	"os"
	"time"
	"tss-demo/api"
	"tss-demo/logger"
	"tss-demo/tss"
)

// NodeConfig holds the configuration for a TSS node.
type NodeConfig struct {
	ID        uint16
	GRPCPort  string
	HTTPPort  string
	PeerAddrs map[uint16]string // map of peer ID to their gRPC address
}

// StartNode initializes and starts the TSS node.
func StartNode(config NodeConfig) {
	log := &logger.StdLogger{}
	node := tss.NewTSSNode(config.ID, log)
	handler := api.NewApiHandler(node)

	startGRPCServer(config, node, log)

	// Allow gRPC server time to initialize.
	time.Sleep(2 * time.Second)

	connectToPeers(config, node)
	startHTTPServer(config, handler, log)
}

// startGRPCServer starts the gRPC server in a separate goroutine.
func startGRPCServer(config NodeConfig, node *tss.TSSNode, log *logger.StdLogger) {
	go func() {
		log.Debugf("[%d] Starting gRPC server on port %s", config.ID, config.GRPCPort)
		if err := node.StartServer(config.GRPCPort); err != nil {
			log.Errorf("[%d] Failed to start gRPC server: %v", config.ID, err)
			os.Exit(1)
		}
	}()
}

// connectToPeers concurrently connects to all peers with retry logic.
func connectToPeers(config NodeConfig, node *tss.TSSNode) {
	for peerID, addr := range config.PeerAddrs {
		peerID, addr := peerID, addr // capture loop variables
		go func() {
			node.ConnectToPeerWithRetry(peerID, addr)
		}()
	}
}

// startHTTPServer sets up and starts the HTTP server in a new goroutine.
func startHTTPServer(config NodeConfig, handler *api.ApiHandler, log *logger.StdLogger) {
	mux := http.NewServeMux()
	mux.HandleFunc("/wallet/create", handler.CreateWallet)
	mux.HandleFunc("/wallet/sign", handler.Sign)

	server := &http.Server{
		Addr:    config.HTTPPort,
		Handler: mux,
	}

	go func() {
		log.Debugf("[%d] Starting HTTP server on port %s", config.ID, config.HTTPPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Errorf("[%d] Failed to start HTTP server: %v", config.ID, err)
			os.Exit(1)
		}
	}()
}

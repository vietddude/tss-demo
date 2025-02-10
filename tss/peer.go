package tss

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	pb "tss-demo/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Peer represents a connection to a remote TSS node.
type Peer struct {
	id      uint16
	client  pb.TSSServiceClient
	stream  pb.TSSService_ExchangeClient
	muPeers sync.Mutex
}

// ConnectToPeer establishes a connection to another TSS node.
func (n *TSSNode) ConnectToPeer(peerID uint16, address string) error {
	if address == "" {
		return errors.New("address cannot be empty")
	}

	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to dial: %w", err)
	}

	client := pb.NewTSSServiceClient(conn)
	stream, err := client.Exchange(context.Background())
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to create stream: %w", err)
	}

	newPeer := &Peer{
		id:     peerID,
		client: client,
		stream: stream,
	}

	n.addPeer(peerID, newPeer)
	go n.listenToPeer(peerID, newPeer)
	return nil
}

// addPeer safely adds a new peer to the node.
func (n *TSSNode) addPeer(peerID uint16, peer *Peer) {
	n.muPeers.Lock()
	defer n.muPeers.Unlock()
	n.peers[peerID] = peer
}

// listenToPeer listens for incoming messages from the peer in a separate goroutine.
func (n *TSSNode) listenToPeer(peerID uint16, peer *Peer) {
	for {
		msg, err := peer.stream.Recv()
		if err != nil {
			n.logger.Errorf("Stream error from peer %d: %v", peerID, err)
			n.removePeerStream(peerID, peer)
			return
		}
		n.party.OnMsg(msg.Payload, uint16(msg.From), msg.Broadcast)
	}
}

// removePeerStream nils out the stream for the given peer.
func (n *TSSNode) removePeerStream(peerID uint16, peer *Peer) {
	n.muPeers.Lock()
	defer n.muPeers.Unlock()
	if p, exists := n.peers[peerID]; exists && p.stream == peer.stream {
		peer.muPeers.Lock()
		peer.stream = nil
		peer.muPeers.Unlock()
	}
}

// ConnectToPeerWithRetry attempts to connect to a peer with retry logic.
func (n *TSSNode) ConnectToPeerWithRetry(peerID uint16, address string) {
	const (
		maxRetries = 5
		retryDelay = 2 * time.Second
	)

	for i := 0; i < maxRetries; i++ {
		err := n.ConnectToPeer(peerID, address)
		if err == nil {
			n.logger.Debugf("[%d] Successfully connected to peer %d at %s", n.nodeID, peerID, address)
			return
		}
		n.logger.Debugf("Failed to connect to peer %d at %s: %v", peerID, address, err)
		if i < maxRetries-1 {
			n.logger.Debugf("Retrying... (%d/%d)", i+1, maxRetries)
			time.Sleep(retryDelay)
		}
	}
	n.logger.Errorf("Unable to connect to peer %d at %s after %d retries", peerID, address, maxRetries)
}

// Close cleans up all peer connections.
func (n *TSSNode) Close() error {
	n.muPeers.Lock()
	defer n.muPeers.Unlock()

	var errs []error
	for id, peer := range n.peers {
		peer.muPeers.Lock()
		if peer.stream != nil {
			if err := peer.stream.CloseSend(); err != nil {
				errs = append(errs, fmt.Errorf("failed to close stream for peer %d: %w", id, err))
			}
		}
		peer.muPeers.Unlock()
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors during cleanup: %v", errs)
	}
	return nil
}

package tss

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
	pb "tss-demo/proto"

	"golang.org/x/crypto/sha3"
	"google.golang.org/grpc"
)

func publicKeyToAddress(publicKey []byte) string {
	hash := sha3.NewLegacyKeccak256()
	hash.Write(publicKey)
	return "0x" + hex.EncodeToString(hash.Sum(nil)[12:]) // Take last 20 bytes
}

// TSSNode represents a node in the TSS network
type TSSNode struct {
	pb.UnimplementedTSSServiceServer
	nodeID     uint16
	party      *Party
	peers      map[uint16]*peer
	grpcServer *grpc.Server
	logger     Logger
	keyGenCh   chan *pb.KeyGenRequest
	mu         sync.RWMutex // Protect peers map
}

type peer struct {
	id     uint16
	client pb.TSSServiceClient
	stream pb.TSSService_ExchangeClient // Changed from ExchangeServer to ExchangeClient
	mu     sync.Mutex
}

// API request/response structures
type CreateWalletRequest struct {
	Threshold int      `json:"threshold"`
	PartyIDs  []uint16 `json:"party_ids"`
}

type CreateWalletResponse struct {
	Address   string `json:"address"`
	PublicKey string `json:"public_key"`
	ShareData string `json:"share_data"`
}

type SignRequest struct {
	Message []byte `json:"message"`
}

type SignResponse struct {
	Signature []byte `json:"signature"`
}

// NewTSSNode creates a new TSS node
func NewTSSNode(nodeID uint16, logger Logger) *TSSNode {
	if logger == nil {
		panic("logger cannot be nil")
	}

	return &TSSNode{
		logger:   logger,
		nodeID:   nodeID,
		party:    NewParty(nodeID, logger),
		peers:    make(map[uint16]*peer),
		keyGenCh: make(chan *pb.KeyGenRequest, 100), // Buffered channel to prevent blocking
	}
}

// StartServer starts the gRPC server
func (n *TSSNode) StartServer(port string) error {
	if port == "" {
		return errors.New("port cannot be empty")
	}

	lis, err := net.Listen("tcp", port)
	if err != nil {
		return err
	}

	n.grpcServer = grpc.NewServer()
	pb.RegisterTSSServiceServer(n.grpcServer, n)

	// Start KeyGen monitoring in background
	go n.monitorKeyGen()

	return n.grpcServer.Serve(lis)
}

// ConnectToPeer establishes connection to another TSS node
func (n *TSSNode) ConnectToPeer(peerID uint16, address string) error {
	if address == "" {
		return errors.New("address cannot be empty")
	}

	conn, err := grpc.Dial(address, grpc.WithInsecure())
	if err != nil {
		return fmt.Errorf("failed to dial: %w", err)
	}

	client := pb.NewTSSServiceClient(conn)

	// Establish bidirectional stream
	stream, err := client.Exchange(context.Background())
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to create stream: %w", err)
	}

	newPeer := &peer{
		id:     peerID,
		client: client,
		stream: stream,
		mu:     sync.Mutex{},
	}

	n.mu.Lock()
	n.peers[peerID] = newPeer
	n.mu.Unlock()

	// Start goroutine to handle incoming messages
	go func() {
		for {
			msg, err := stream.Recv()
			if err != nil {
				n.logger.Errorf("Stream error from peer %d: %v", peerID, err)
				n.mu.Lock()
				if p, exists := n.peers[peerID]; exists && p.stream == stream {
					p.mu.Lock()
					p.stream = nil
					p.mu.Unlock()
				}
				n.mu.Unlock()
				return
			}

			n.party.OnMsg(msg.Payload, uint16(msg.From), msg.Broadcast)
		}
	}()

	return nil
}

// Cleanup function to close connections
func (n *TSSNode) Close() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	var errs []error
	for id, peer := range n.peers {
		peer.mu.Lock()
		if peer.stream != nil {
			if err := peer.stream.CloseSend(); err != nil {
				errs = append(errs, fmt.Errorf("failed to close stream for peer %d: %w", id, err))
			}
		}
		peer.mu.Unlock()
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors during cleanup: %v", errs)
	}
	return nil
}

func (n *TSSNode) ConnectToPeerWithRetry(peerID uint16, address string) {
	maxRetries := 5
	retryDelay := 2 * time.Second

	for i := 0; i < maxRetries; i++ {
		err := n.ConnectToPeer(peerID, address)
		if err == nil {
			n.logger.Debugf("Successfully connected to peer %d at %s", peerID, address)
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

// StartKeyGen implements the gRPC method
func (n *TSSNode) StartKeyGen(ctx context.Context, req *pb.KeyGenRequest) (*pb.KeyGenResponse, error) {
	if req == nil {
		return nil, errors.New("request cannot be nil")
	}

	n.logger.Debugf("Node %d received KeyGen request", n.nodeID)

	select {
	case n.keyGenCh <- req:
		return &pb.KeyGenResponse{Success: true}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Exchange implements the gRPC service
func (n *TSSNode) Exchange(stream pb.TSSService_ExchangeServer) error {
	for {
		msg, err := stream.Recv()
		if err != nil {
			return err
		}

		peerID := uint16(msg.From)
		// n.mu.Lock()
		// if peer, exists := n.peers[peerID]; exists {
		// 	peer.mu.Lock()
		// 	// Here we store the server-side stream separately or handle it differently
		// 	// depending on your needs
		// 	peer.mu.Unlock()
		// }
		// n.mu.Unlock()

		n.party.OnMsg(msg.Payload, peerID, msg.Broadcast)
	}
}

// CreateWallet initiates the DKG process
func (n *TSSNode) CreateWallet(w http.ResponseWriter, r *http.Request) {
	if r.Body == nil {
		http.Error(w, "Request body is empty", http.StatusBadRequest)
		return
	}

	var req CreateWalletRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Threshold <= 0 || len(req.PartyIDs) == 0 {
		http.Error(w, "Invalid threshold or empty party IDs", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Notify peers
	n.mu.RLock()
	for peerID, peer := range n.peers {
		n.logger.Debugf("Notifying peer %d", peerID)
		_, err := peer.client.StartKeyGen(ctx, &pb.KeyGenRequest{
			Threshold: int32(req.Threshold),
			PartyIds:  convertToUint32(req.PartyIDs),
		})
		if err != nil {
			n.mu.RUnlock()
			n.logger.Errorf("Failed to notify peer %d: %v", peerID, err)
			http.Error(w, "Failed to coordinate with peers", http.StatusInternalServerError)
			return
		}
	}
	n.mu.RUnlock()

	// Initialize party
	n.party.Init(req.PartyIDs, req.Threshold, n.sendMessage)

	// Run DKG
	shareData, err := n.party.KeyGen(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	n.party.SetShareData(shareData)

	pubKey, err := n.party.ThresholdPK()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	n.logger.Debugf("Wallet created, address: %s",
		pubKey,
		publicKeyToAddress(pubKey),
	)

	resp := CreateWalletResponse{
		Address:   publicKeyToAddress(pubKey),
		PublicKey: hex.EncodeToString(pubKey),
		ShareData: base64.StdEncoding.EncodeToString(shareData),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		n.logger.Errorf("Failed to encode response: %v", err)
	}
}

// Sign initiates the signing process
func (n *TSSNode) Sign(w http.ResponseWriter, r *http.Request) {
	if r.Body == nil {
		http.Error(w, "Request body is empty", http.StatusBadRequest)
		return
	}

	var req SignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if len(req.Message) == 0 {
		http.Error(w, "Message cannot be empty", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	signature, err := n.party.Sign(ctx, req.Message)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := SignResponse{
		Signature: signature,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		n.logger.Errorf("Failed to encode response: %v", err)
	}
}

func (n *TSSNode) sendMessage(msg []byte, isBroadcast bool, to uint16) {
	tssMsg := &pb.TSSMessage{
		From:      uint32(n.nodeID),
		Payload:   msg,
		Broadcast: isBroadcast,
	}

	n.mu.RLock()
	defer n.mu.RUnlock()

	if isBroadcast {
		n.logger.Debugf("Broadcasting message from %d", n.nodeID)
		for peerID, peer := range n.peers {
			peer.mu.Lock()
			if peer.stream != nil {
				if err := peer.stream.Send(tssMsg); err != nil {
					n.logger.Errorf("Error broadcasting to peer %d: %v", peerID, err)
				}
			} else {
				n.logger.Warnf("Peer %d has no stream", peerID)
			}
			peer.mu.Unlock()
		}
	} else {
		if peer, exists := n.peers[to]; exists {
			peer.mu.Lock()
			if peer.stream != nil {
				if err := peer.stream.Send(tssMsg); err != nil {
					n.logger.Errorf("Error sending to peer %d: %v", to, err)
				}
			} else {
				n.logger.Warnf("Peer %d has no stream", to)
			}
			peer.mu.Unlock()
		} else {
			n.logger.Warnf("Peer %d not found", to)
		}
	}
}

func (n *TSSNode) monitorKeyGen() {
	for req := range n.keyGenCh {
		n.logger.Debugf("Node %d starting KeyGen process", n.nodeID)

		partyIds := convertToUint16(req.PartyIds)

		n.party.Init(partyIds, int(req.Threshold), n.sendMessage)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		shareData, err := n.party.KeyGen(ctx)
		cancel()

		if err != nil {
			n.logger.Errorf("KeyGen failed: %v", err)
			continue
		}

		n.logger.Debugf("Node %d completed KeyGen, share size: %d bytes", n.nodeID, len(shareData))
	}
}

func convertToUint16(ids []uint32) []uint16 {
	result := make([]uint16, len(ids))
	for i, id := range ids {
		result[i] = uint16(id)
	}
	return result
}

func convertToUint32(ids []uint16) []uint32 {
	result := make([]uint32, len(ids))
	for i, id := range ids {
		result[i] = uint32(id)
	}
	return result
}

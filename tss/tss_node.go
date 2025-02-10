package tss

import (
	"context"
	"errors"
	"net"
	"sync"
	pb "tss-demo/proto"
	"tss-demo/session"

	"google.golang.org/grpc"
)

// TSSNode represents a node in the TSS network
type TSSNode struct {
	pb.UnimplementedTSSServiceServer
	nodeID         uint16
	party          *Party
	peers          map[uint16]*Peer
	grpcServer     *grpc.Server
	logger         Logger
	keyGenCh       chan *pb.KeyGenRequest
	signCh         chan *pb.SignRequest
	muPeers        sync.RWMutex // Protect peers map
	sessionManager *session.SessionManager
}

// NewTSSNode creates a new TSS node
func NewTSSNode(nodeID uint16, logger Logger) *TSSNode {
	if logger == nil {
		panic("logger cannot be nil")
	}

	return &TSSNode{
		logger:         logger,
		nodeID:         nodeID,
		party:          NewParty(nodeID, logger),
		peers:          make(map[uint16]*Peer),
		keyGenCh:       make(chan *pb.KeyGenRequest, 100), // Buffered channel to prevent blocking
		signCh:         make(chan *pb.SignRequest, 100),   // Buffered channel to prevent blocking
		sessionManager: session.NewSessionManager(),
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
	// Start Sign monitoring in background
	go n.monitorSign()

	return n.grpcServer.Serve(lis)
}

// StartKeyGen implements the gRPC method
func (n *TSSNode) StartKeyGen(ctx context.Context, req *pb.KeyGenRequest) (*pb.KeyGenResponse, error) {
	if req == nil || req.SessionId == "" {
		return nil, errors.New("invalid request")
	}

	// Tạo session nếu chưa tồn tại
	session, exists := n.sessionManager.GetSession(req.SessionId)
	if !exists {
		session = n.sessionManager.CreateSession(req.SessionId, len(n.peers)+1)
	}

	// Thêm node hiện tại vào session
	session.AddParty(n.nodeID)

	// Kiểm tra nếu session đã sẵn sàng
	if !session.IsReady() {
		n.logger.Debugf("[%d] Session %s is waiting for more parties", n.nodeID, req.SessionId)
	}

	// Gửi yêu cầu vào channel xử lý KeyGen
	select {
	case n.keyGenCh <- req:
		return &pb.KeyGenResponse{Success: true}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// StartSign implements the gRPC method
func (n *TSSNode) StartSign(ctx context.Context, req *pb.SignRequest) (*pb.SignResponse, error) {
	if req == nil {
		return nil, errors.New("request cannot be nil")
	}

	// n.logger.Debugf("Node %d received Sign request", n.nodeID)
	// Tạo session nếu chưa tồn tại
	session, exists := n.sessionManager.GetSession(req.SessionId)
	if !exists {
		session = n.sessionManager.CreateSession(req.SessionId, len(n.peers)+1)
	}

	// Thêm node hiện tại vào session
	session.AddParty(n.nodeID)

	// Kiểm tra nếu session đã sẵn sàng
	if !session.IsReady() {
		n.logger.Debugf("[%d] Session %s is waiting for more parties", n.nodeID, req.SessionId)
	}

	select {
	case n.signCh <- req:
		return &pb.SignResponse{Success: true}, nil
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
		n.party.OnMsg(msg.Payload, peerID, msg.Broadcast)
	}
}

func (n *TSSNode) sendMessage(msg []byte, isBroadcast bool, to uint16) {
	tssMsg := &pb.TSSMessage{
		From:      uint32(n.nodeID),
		Payload:   msg,
		Broadcast: isBroadcast,
	}

	n.muPeers.RLock()
	defer n.muPeers.RUnlock()

	if isBroadcast {
		n.logger.Debugf("[%d] Broadcasting message", n.nodeID)
		for peerID, peer := range n.peers {
			peer.muPeers.Lock()
			if peer.stream != nil {
				if err := peer.stream.Send(tssMsg); err != nil {
					n.logger.Errorf("Error broadcasting to peer %d: %v", peerID, err)
				}
			} else {
				n.logger.Warnf("Peer %d has no stream", peerID)
			}
			peer.muPeers.Unlock()
		}
	} else {
		if peer, exists := n.peers[to]; exists {
			peer.muPeers.Lock()
			if peer.stream != nil {
				if err := peer.stream.Send(tssMsg); err != nil {
					n.logger.Errorf("Error sending to peer %d: %v", to, err)
				}
			} else {
				n.logger.Warnf("Peer %d has no stream", to)
			}
			peer.muPeers.Unlock()
		} else {
			n.logger.Warnf("Peer %d not found", to)
		}
	}
}

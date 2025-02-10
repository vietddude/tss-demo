package tss

import (
	"context"
	"fmt"
	"time"
	"tss-demo/db"
	pb "tss-demo/proto"
	"tss-demo/utils"
)

// CreateWallet initiates the DKG process
func (n *TSSNode) CreateWallet(ctx context.Context, threshold int, partyIDs []uint16, sessionID string) ([]byte, []byte, error) {
	// Notify peers
	n.muPeers.RLock()
	for peerID, peer := range n.peers {
		n.logger.Debugf("[%d] Notifying peer %d", n.nodeID, peerID)
		_, err := peer.client.StartKeyGen(ctx, &pb.KeyGenRequest{
			Threshold: int32(threshold),
			PartyIds:  utils.ConvertToUint32(partyIDs),
			SessionId: sessionID,
		})
		if err != nil {
			n.muPeers.RUnlock()
			n.logger.Errorf("[%d] Failed to notify peer %d: %v", n.nodeID, peerID, err)
			return nil, nil, err
		}
	}
	n.muPeers.RUnlock()

	// Initialize party
	n.party.Init(partyIDs, threshold, n.sendMessage)

	// Run DKG
	shareData, err := n.party.KeyGen(ctx)
	if err != nil {
		return nil, nil, err
	}

	n.party.SetShareData(shareData)
	db.SaveToJSON(shareData, fmt.Sprintf("share_%d_%s.json", n.nodeID, sessionID))
	pubKey, err := n.party.ThresholdPK()
	if err != nil {
		return nil, nil, err
	}

	n.logger.Debugf("Wallet created, address: %s",
		utils.PublicKeyToAddress(pubKey),
	)

	return pubKey, shareData, nil
}

// Listen for KeyGen requests of other nodes
func (n *TSSNode) monitorKeyGen() {
	for req := range n.keyGenCh {
		n.logger.Debugf("[%d] Starting KeyGen process", n.nodeID)
		sessionID := req.SessionId
		// Tạo session nếu chưa tồn tại
		session, exists := n.sessionManager.GetSession(sessionID)
		if !exists {
			session = n.sessionManager.CreateSession(req.SessionId, len(n.peers)+1)
		}

		// Thêm node hiện tại vào session
		session.AddParty(n.nodeID)

		// Kiểm tra nếu session đã sẵn sàng
		if !session.IsReady() {
			n.logger.Debugf("[%d] Session %s is waiting for more parties", n.nodeID, req.SessionId)
		}

		partyIds := utils.ConvertToUint16(req.PartyIds)

		n.party.Init(partyIds, int(req.Threshold), n.sendMessage)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		shareData, err := n.party.KeyGen(ctx)
		cancel()
		db.SaveToJSON(shareData, fmt.Sprintf("share_%d_%s.json", n.nodeID, sessionID))

		if err != nil {
			n.logger.Errorf("KeyGen failed: %v", err)
			continue
		}

		n.logger.Debugf("[%d] completed KeyGen, share size: %d bytes", n.nodeID, len(shareData))
	}
}

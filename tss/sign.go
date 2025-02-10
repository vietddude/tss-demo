package tss

import (
	"context"
	"fmt"
	"time"
	"tss-demo/db"
	pb "tss-demo/proto"
	"tss-demo/utils"
)

// Sign initiates the signing process
func (n *TSSNode) Sign(ctx context.Context, message []byte, partyIds []uint16, sessionID string) ([]byte, error) {
	// Notify peers
	n.muPeers.RLock()
	for peerID, peer := range n.peers {
		n.logger.Debugf("[%d] Notifying peer %d", n.nodeID, peerID)
		_, err := peer.client.StartSign(ctx, &pb.SignRequest{
			Message:   message,
			SessionId: sessionID,
			PartyIds:  utils.ConvertToUint32(partyIds),
		})
		if err != nil {
			n.muPeers.RUnlock()
			n.logger.Errorf("[%d] Failed to notify peer %d: %v", n.nodeID, peerID, err)
			return nil, err
		}
		// Only notify one peer.
		// break
	}
	n.muPeers.RUnlock()

	shareData, err := db.LoadFromJSON(fmt.Sprintf("share_%d_%s.json", n.nodeID, sessionID))
	if err != nil {
		n.logger.Errorf("[%d] Failed to load share data: %v", n.nodeID, err)
		return nil, err
	}
	n.party.Init(partyIds, int(2), n.sendMessage)
	n.party.SetShareData(shareData)
	signature, err := n.party.Sign(ctx, message)

	if err != nil {
		return nil, err
	}
	return signature, nil
}

func (n *TSSNode) monitorSign() {
	for req := range n.signCh {
		n.logger.Debugf("[%d] Starting Sign process", n.nodeID)
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

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

		shareData, err := db.LoadFromJSON(fmt.Sprintf("share_%d_%s.json", n.nodeID, req.SessionId))
		if err != nil {
			n.logger.Errorf("[%d] Failed to load share data: %v", n.nodeID, err)
			cancel()
			continue
		}
		partyIds := utils.ConvertToUint16(req.PartyIds)
		n.party.Init(partyIds, int(2), n.sendMessage)
		n.party.SetShareData(shareData)
		signature, err := n.party.Sign(ctx, req.Message)
		cancel()
		if err != nil {
			n.logger.Errorf("[%d] Failed to sign: %v", n.nodeID, err)
			continue
		}

		n.logger.Debugf("[%d] Signature: %s", n.nodeID, signature)
	}
}

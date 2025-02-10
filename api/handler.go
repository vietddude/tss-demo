package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"tss-demo/tss"
	"tss-demo/utils"
)

// API request/response structures
type CreateWalletRequest struct {
	Threshold int      `json:"threshold"`
	PartyIDs  []uint16 `json:"party_ids"`
	SessionID string   `json:"session_id"`
}

type CreateWalletResponse struct {
	PartyID       uint16 `json:"party_id"`
	PublicAddress string `json:"public_address"`
	ShareData     string `json:"share_data"`
}

type SignRequest struct {
	Message   []byte   `json:"message"`
	PartyIDs  []uint16 `json:"party_ids"`
	SessionID string   `json:"session_id"`
}

type SignResponse struct {
	Signature []byte `json:"signature"`
}

type ApiHandler struct {
	tssNode *tss.TSSNode
}

func NewApiHandler(node *tss.TSSNode) *ApiHandler {
	return &ApiHandler{tssNode: node}
}

// CreateWallet creates a new wallet with the given threshold and party IDs
func (h *ApiHandler) CreateWallet(w http.ResponseWriter, r *http.Request) {
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

	pubKey, shareData, err := h.tssNode.CreateWallet(ctx, req.Threshold, req.PartyIDs, req.SessionID)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := CreateWalletResponse{
		PartyID:       req.PartyIDs[0],
		PublicAddress: utils.PublicKeyToAddress(pubKey),
		ShareData:     base64.StdEncoding.EncodeToString(shareData),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		fmt.Printf("Error encoding response: %v\n", err)
	}
}

// Sign signs a transaction with the given wallet address and message
func (h *ApiHandler) Sign(w http.ResponseWriter, r *http.Request) {

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
	signature, err := h.tssNode.Sign(ctx, req.Message, req.PartyIDs, req.SessionID)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := SignResponse{
		Signature: signature,
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		fmt.Printf("Error encoding response: %v\n", err)
	}
}

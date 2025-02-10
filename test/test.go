package main

import (
	"context"
	"encoding/asn1"
	"encoding/base64"
	"fmt"
	"log"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

func main() {
	client, err := ethclient.Dial("wss://sepolia.infura.io/ws/v3/6c89fb7fa351451f939eea9da6bee755")
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}

	fromAddress := common.HexToAddress("0x03B64c614Bfa6F731c3a790921c88513B2CEC777")
	nonce, err := client.PendingNonceAt(context.Background(), fromAddress)
	if err != nil {
		log.Fatalf("Failed to retrieve nonce: %v", err)
	}

	toAddress := common.HexToAddress("0x7ad87583Ba7AD9Ab682fB7fec31e14Cf3bbf4804")
	value := big.NewInt(1000000000000000000)
	gasLimit := uint64(21000)
	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		log.Fatalf("Failed to suggest gas price: %v", err)
	}

	tx := types.NewTransaction(nonce, toAddress, value, gasLimit, gasPrice, nil)
	txBytes, err := tx.MarshalBinary()
	if err != nil {
		log.Fatalf("Failed to encode transaction: %v", err)
	}
	fmt.Printf("Transaction encoded: %x\n", txBytes)

	signature, err := signWithTSS(txBytes)
	if err != nil {
		log.Fatalf("Failed to sign transaction with TSS: %v", err)
	}

	chainID := big.NewInt(11155111)
	signedTx, err := createSignedTransaction(tx, signature, chainID)
	if err != nil {
		log.Fatalf("Failed to create signed transaction: %v", err)
	}

	fmt.Printf("Transaction sent: %s\n", signedTx.Hash().Hex())
}

func ConvertFromASN1(sigASN1 []byte) (*big.Int, *big.Int, error) {
	var sig struct {
		R, S *big.Int
	}

	_, err := asn1.Unmarshal(sigASN1, &sig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal ASN.1 signature: %w", err)
	}

	return sig.R, sig.S, nil
}

func signWithTSS(data []byte) ([]byte, error) {
	sigBase64 := "MEQCIBCSU6LuB2fHwhOMMwkgYaBKXNJ2sMmRQkUdLGkDJv5KAiB96flFemW6h6UwaW/qJ7ZYlPOjIIWDUWo0E1DjAXRnBw=="
	sigBytes, err := base64.StdEncoding.DecodeString(sigBase64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 signature: %w", err)
	}

	r, s, err := ConvertFromASN1(sigBytes)
	if err != nil {
		return nil, err
	}

	v := byte(27) // Giả sử giá trị V mặc định

	sigRaw := make([]byte, 65)
	copy(sigRaw[0:32], r.Bytes())
	copy(sigRaw[32:64], s.Bytes())
	sigRaw[64] = v

	return sigRaw, nil
}

func createSignedTransaction(tx *types.Transaction, signature []byte, chainID *big.Int) (*types.Transaction, error) {
	if len(signature) != 65 {
		return nil, fmt.Errorf("invalid signature length: %d", len(signature))
	}

	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:64])
	v := new(big.Int).SetBytes([]byte{signature[64]})

	signedTx := types.NewTx(&types.LegacyTx{
		Nonce:    tx.Nonce(),
		To:       tx.To(),
		Value:    tx.Value(),
		Gas:      tx.Gas(),
		GasPrice: tx.GasPrice(),
		Data:     tx.Data(),
		V:        v,
		R:        r,
		S:        s,
	})

	return signedTx, nil
}

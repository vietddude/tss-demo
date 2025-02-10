package utils

import (
	"encoding/hex"

	"golang.org/x/crypto/sha3"
)

func PublicKeyToAddress(publicKey []byte) string {
	hash := sha3.NewLegacyKeccak256()
	hash.Write(publicKey)
	return "0x" + hex.EncodeToString(hash.Sum(nil)[12:]) // Take last 20 bytes
}

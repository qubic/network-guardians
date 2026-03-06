package schnorrq

import (
	"encoding/hex"
	"errors"

	"github.com/linckode/circl/ecc/fourq"
	"github.com/linckode/circl/xof/k12"
)

const (
	signatureLen = 64 // signature is 64 bytes
)

var (
	ErrInvalidSignatureLength = errors.New("invalid signature length: expected 64 bytes")
	ErrInvalidMessageHex      = errors.New("invalid message hex encoding")
	ErrInvalidSignatureHex    = errors.New("invalid signature hex encoding")
	ErrSignatureVerifyFailed  = errors.New("signature verification failed")
	ErrInvalidPublicKey       = errors.New("invalid public key format")
	ErrInvalidSignaturePoint  = errors.New("invalid signature point")
)

// verifies SchnorrQ signatures using FourQ curve
type Verifier struct{}

func NewVerifier() *Verifier {
	return &Verifier{}
}

func (v *Verifier) Verify(operator, messageHex, signatureHex string) (bool, error) {
	// Decode operator identity to public key
	pubKey, err := DecodeIdentity(operator)
	if err != nil {
		return false, err
	}

	// Decode message from hex
	message, err := hex.DecodeString(messageHex)
	if err != nil {
		return false, ErrInvalidMessageHex
	}

	// Hash the message to get 32-byte digest using K12
	messageDigest := hashK12_32(message)

	// Decode signature from hex
	signature, err := hex.DecodeString(signatureHex)
	if err != nil {
		return false, ErrInvalidSignatureHex
	}

	if len(signature) != signatureLen {
		return false, ErrInvalidSignatureLength
	}

	// Convert to fixed-size arrays
	var sig [64]byte
	copy(sig[:], signature)

	// Verify using SchnorrQ algorithm
	if err := verifySchnorrQ(pubKey, messageDigest, sig); err != nil {
		return false, err
	}

	return true, nil
}

// Verification:
//  1. Decode public key P
//  2. Compute e = K12(R || P || messageDigest)[0:32]
//  3. Compute s*G + e*P using DoubleScalarMult
//  4. Verify result equals R
func verifySchnorrQ(pubKey [32]byte, messageDigest [32]byte, signature [64]byte) error {
	if (pubKey[15]&0x80 != 0) || (signature[15]&0x80 != 0) || (signature[62]&0xC0 != 0) || signature[63] != 0 {
		return ErrInvalidPublicKey
	}

	// Decode public key point
	var point fourq.Point
	if !point.Unmarshal(&pubKey) {
		return ErrInvalidPublicKey
	}

	// Prepare temp buffer
	var temp [96]byte
	copy(temp[:32], signature[:32])   // R
	copy(temp[32:64], pubKey[:])      // pubKey
	copy(temp[64:], messageDigest[:]) // messageDigest

	tempHash := hashK12_64(temp[:])

	// Extract scalars
	var s [32]byte
	copy(s[:], signature[32:64])

	var e [32]byte
	copy(e[:], tempHash[:32])

	// Compute s*G + e*P using DoubleScalarMult
	point.DoubleScalarMult(&s, &point, &e)

	// Encode the result
	var encoded [32]byte
	point.Marshal(&encoded)

	// Compare with R from signature
	var R [32]byte
	copy(R[:], signature[:32])

	if encoded != R {
		return ErrSignatureVerifyFailed
	}

	return nil
}

// hashK12_32 computes a 32-byte K12 hash of data.
func hashK12_32(data []byte) [32]byte {
	h := k12.NewDraft10(nil)
	h.Write(data)

	var out [32]byte
	h.Read(out[:])
	return out
}

// hashK12_64 computes a 64-byte K12 hash of data.
func hashK12_64(data []byte) [64]byte {
	h := k12.NewDraft10(nil)
	h.Write(data)

	var out [64]byte
	h.Read(out[:])
	return out
}

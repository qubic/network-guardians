package schnorrq

import (
	"encoding/binary"
	"errors"
	"strings"

	"github.com/linckode/circl/xof/k12"
)

const (
	identityLen  = 60
	publicKeyLen = 32
)

var (
	ErrInvalidIdentityLength = errors.New("invalid identity length: expected 60 characters")
	ErrInvalidIdentityChar   = errors.New("invalid character in identity: must be A-Z or a-z")
	ErrChecksumMismatch      = errors.New("identity checksum mismatch")
)

func DecodeIdentity(identity string) ([32]byte, error) {
	var pubKey [32]byte

	// Normalize to uppercase
	identity = strings.ToUpper(identity)

	if len(identity) != identityLen {
		return pubKey, ErrInvalidIdentityLength
	}

	// Validate characters
	for i := 0; i < identityLen; i++ {
		if identity[i] < 'A' || identity[i] > 'Z' {
			return pubKey, ErrInvalidIdentityChar
		}
	}

	for i := 0; i < 4; i++ {
		// (little-endian base-26)
		var value uint64
		for j := 13; j >= 0; j-- {
			charVal := uint64(identity[i*14+j] - 'A')
			value = value*26 + charVal
		}

		// Store as little-endian 8 bytes
		binary.LittleEndian.PutUint64(pubKey[i*8:(i+1)*8], value)
	}

	// Verify checksum (last 4 characters)
	expectedChecksum := identity[56:60]
	computedChecksum := computeK12Checksum(pubKey)

	if computedChecksum != expectedChecksum {
		return pubKey, ErrChecksumMismatch
	}

	return pubKey, nil
}

func computeK12Checksum(pubKey [32]byte) string {
	h := k12.NewDraft10([]byte{})
	h.Write(pubKey[:])

	var checksumBytes [3]byte
	h.Read(checksumBytes[:])

	// Combine into 18-bit value
	checksumInt := uint64(checksumBytes[0]) |
		(uint64(checksumBytes[1]) << 8) |
		(uint64(checksumBytes[2]) << 16)
	checksumInt &= 0x3FFFF // 18 bits

	// Convert to 4 base-26 characters
	checksum := make([]byte, 4)
	for i := 0; i < 4; i++ {
		checksum[i] = byte('A' + (checksumInt % 26))
		checksumInt /= 26
	}

	return string(checksum)
}

// EncodeIdentity converts a 32-byte public key to a Qubic identity string
func EncodeIdentity(pubKey [32]byte) string {
	identity := make([]byte, 60)

	// Encode 32 bytes as 56 base-26 characters
	for i := 0; i < 4; i++ {
		value := binary.LittleEndian.Uint64(pubKey[i*8 : (i+1)*8])

		// Convert to 14 base-26 digits (little-endian order)
		for j := 0; j < 14; j++ {
			identity[i*14+j] = byte('A' + (value % 26))
			value /= 26
		}
	}

	// Append checksum
	checksum := computeK12Checksum(pubKey)
	copy(identity[56:], checksum)

	return string(identity)
}

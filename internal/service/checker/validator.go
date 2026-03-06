package checker

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"time"

	"github.com/qubic/network-guardians/internal/config"
	"github.com/qubic/network-guardians/internal/crypto/schnorrq"
	"github.com/qubic/network-guardians/internal/domain"
)

// validateNodeIP rejects private, loopback, link-local, and other non-routable IPs
func validateNodeIP(ip string) error {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return &ValidationError{Reason: "invalid_ip"}
	}
	if parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsLinkLocalUnicast() ||
		parsed.IsLinkLocalMulticast() || parsed.IsUnspecified() || parsed.IsMulticast() {
		return &ValidationError{Reason: "blocked_ip"}
	}
	return nil
}

// ValidationError represents a validation failure
type ValidationError struct {
	Reason string
}

func (e *ValidationError) Error() string {
	return e.Reason
}

// LiteNodeResponse represents the response from a lite node
type LiteNodeResponse struct {
	Tick      uint32            `json:"tick"`
	Epoch     uint16            `json:"epoch"`
	ExtraInfo LiteNodeExtraInfo `json:"extraInfo"`
}

// LiteNodeExtraInfo contains the signed message data
type LiteNodeExtraInfo struct {
	Type       string `json:"type"`
	Version    string `json:"version"`
	Alias      string `json:"alias"`
	Uptime     int64  `json:"uptime"`
	Timestamp  int64  `json:"timestamp"` // Unix timestamp in seconds
	MessageHex string `json:"messageHex"`
	Signature  string `json:"signature"`
	Operator   string `json:"operator"`
}

// BobNodeResponse represents the response from a bob node
type BobNodeResponse struct {
	CurrentProcessingEpoch   uint16           `json:"currentProcessingEpoch"`
	CurrentFetchingTick      uint32           `json:"currentFetchingTick"`
	CurrentFetchingLogTick   uint32           `json:"currentFetchingLogTick"`
	CurrentVerifyLoggingTick uint32           `json:"currentVerifyLoggingTick"`
	CurrentIndexingTick      uint32           `json:"currentIndexingTick"`
	InitialTick              uint32           `json:"initialTick"`
	BobVersion               string           `json:"bobVersion"`
	BobVersionGitHash        string           `json:"bobVersionGitHash"`
	BobCompiler              string           `json:"bobCompiler"`
	ExtraInfo                BobNodeExtraInfo `json:"extraInfo"`
}

// BobNodeExtraInfo contains the signed message data for bob nodes
type BobNodeExtraInfo struct {
	Type       string `json:"type"`
	Version    string `json:"version"`
	Alias      string `json:"alias"`
	Uptime     int64  `json:"uptime"`
	Timestamp  int64  `json:"timestamp"`
	Operator   string `json:"operator"`
	MessageHex string `json:"messageHex"`
	Signature  string `json:"signature"`
}

// Validator validates node responses
type Validator struct {
	cfg       *config.ScoringConfig
	reference *domain.ReferenceData
	verifier  *schnorrq.Verifier
}

// creates a new validator
func NewValidator(cfg *config.ScoringConfig, reference *domain.ReferenceData) *Validator {
	return &Validator{
		cfg:       cfg,
		reference: reference,
		verifier:  schnorrq.NewVerifier(),
	}
}

// validates a lite node response
func (v *Validator) ValidateLiteNode(operator string, resp *LiteNodeResponse) (float64, error) {
	// 1. Epoch match check
	if err := v.validateEpoch(resp.Epoch); err != nil {
		return 0, err
	}

	// 2. Operator identity check
	if resp.ExtraInfo.Operator != operator {
		return 0, &ValidationError{Reason: "operator_mismatch"}
	}

	// 3. Signature verification (from extraInfo)
	if err := v.validateSignature(operator, resp.ExtraInfo.MessageHex, resp.ExtraInfo.Signature); err != nil {
		return 0, err
	}

	// 4. Extract signed payload and verify every signed field matches outer extraInfo
	signedMsg, err := extractLiteSignedMessage(resp.ExtraInfo.MessageHex)
	if err != nil {
		return 0, err
	}
	if signedMsg.Operator != resp.ExtraInfo.Operator {
		return 0, &ValidationError{Reason: "signed_operator_mismatch"}
	}
	if signedMsg.Type != resp.ExtraInfo.Type {
		return 0, &ValidationError{Reason: "signed_type_mismatch"}
	}
	if signedMsg.Version != resp.ExtraInfo.Version {
		return 0, &ValidationError{Reason: "signed_version_mismatch"}
	}
	if signedMsg.Alias != resp.ExtraInfo.Alias {
		return 0, &ValidationError{Reason: "signed_alias_mismatch"}
	}
	if signedMsg.Uptime != resp.ExtraInfo.Uptime {
		return 0, &ValidationError{Reason: "signed_uptime_mismatch"}
	}
	if signedMsg.Timestamp != resp.ExtraInfo.Timestamp {
		return 0, &ValidationError{Reason: "signed_timestamp_mismatch"}
	}

	// 5. Freshness check against the cryptographically bound timestamp
	if err := v.validateTimestamp(signedMsg.Timestamp); err != nil {
		return 0, err
	}

	// 6. Calculate sync score based on tick difference
	syncScore := v.calculateSyncScore(resp.Tick)

	return syncScore, nil
}

// validates a bob node response
func (v *Validator) ValidateBobNode(operator string, resp *BobNodeResponse) (float64, error) {
	// 1. Epoch match check
	if err := v.validateEpoch(resp.CurrentProcessingEpoch); err != nil {
		return 0, err
	}

	// 2. Operator identity check
	if resp.ExtraInfo.Operator != operator {
		return 0, &ValidationError{Reason: "operator_mismatch"}
	}

	// 3. Signature verification (from extraInfo)
	if err := v.validateSignature(operator, resp.ExtraInfo.MessageHex, resp.ExtraInfo.Signature); err != nil {
		return 0, err
	}

	// 4. Extract signed payload and verify every signed field matches outer extraInfo
	signedMsg, err := extractBobSignedMessage(resp.ExtraInfo.MessageHex)
	if err != nil {
		return 0, err
	}
	if signedMsg.Type != resp.ExtraInfo.Type {
		return 0, &ValidationError{Reason: "signed_type_mismatch"}
	}
	if signedMsg.Version != resp.ExtraInfo.Version {
		return 0, &ValidationError{Reason: "signed_version_mismatch"}
	}
	if normalizeBobAliasForCompare(signedMsg.Alias) != normalizeBobAliasForCompare(resp.ExtraInfo.Alias) {
		return 0, &ValidationError{Reason: "signed_alias_mismatch"}
	}
	if signedMsg.Uptime != resp.ExtraInfo.Uptime {
		return 0, &ValidationError{Reason: "signed_uptime_mismatch"}
	}
	if signedMsg.Timestamp != resp.ExtraInfo.Timestamp {
		return 0, &ValidationError{Reason: "signed_timestamp_mismatch"}
	}

	// 5. Freshness check against the cryptographically bound timestamp
	if err := v.validateTimestamp(signedMsg.Timestamp); err != nil {
		return 0, err
	}

	// 6. Calculate sync score based on tick difference using currentFetchingTick
	syncScore := v.calculateSyncScore(resp.CurrentFetchingTick)

	return syncScore, nil
}

// checks if the timestamp is fresh enough
func (v *Validator) validateTimestamp(timestamp int64) error {
	nodeTime := time.Unix(timestamp, 0)
	age := time.Since(nodeTime)

	maxAge := time.Duration(v.cfg.TimestampMaxAge) * time.Second
	if age > maxAge || age < -maxAge {
		return &ValidationError{Reason: "timestamp_stale"}
	}

	return nil
}

// checks if the node's epoch matches the reference
func (v *Validator) validateEpoch(nodeEpoch uint16) error {
	refEpoch := v.reference.GetEpoch()

	if nodeEpoch != refEpoch {
		return &ValidationError{Reason: fmt.Sprintf("epoch_mismatch:%d!=%d", nodeEpoch, refEpoch)}
	}

	return nil
}

// verifies the SchnorrQ signature
func (v *Validator) validateSignature(operator, messageHex, signature string) error {
	if messageHex == "" || signature == "" {
		return &ValidationError{Reason: "missing_signature_data"}
	}

	valid, err := v.verifier.Verify(operator, messageHex, signature)
	if err != nil {
		return &ValidationError{Reason: fmt.Sprintf("signature_error:%s", err.Error())}
	}

	if !valid {
		return &ValidationError{Reason: "invalid_signature"}
	}

	return nil
}

// liteSignedMessage represents the JSON structure inside a lite node's signed messageHex
type liteSignedMessage struct {
	Alias     string `json:"alias"`
	Timestamp int64  `json:"timestamp"`
	Operator  string `json:"operator"`
	Type      string `json:"type"`
	Uptime    int64  `json:"uptime"`
	Version   string `json:"version"`
}

const bobSignedMessageSize = 80
const bobAliasMaxBytes = 12
const bobTimestampOffset = 40

type bobSignedMessage struct {
	Type      string
	Version   string
	Alias     string
	Uptime    int64
	Timestamp int64
	PubKey    [32]byte
}

// extractLiteSignedMessage decodes and parses the signed JSON payload from lite messageHex
func extractLiteSignedMessage(messageHex string) (*liteSignedMessage, error) {
	raw, err := hex.DecodeString(messageHex)
	if err != nil {
		return nil, &ValidationError{Reason: "signed_message_decode_failed"}
	}

	var msg liteSignedMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil, &ValidationError{Reason: "signed_message_parse_failed"}
	}

	if msg.Timestamp == 0 {
		return nil, &ValidationError{Reason: "signed_message_missing_timestamp"}
	}

	return &msg, nil
}

// extractBobSignedMessage decodes and parses the payload from bob messageHex
func extractBobSignedMessage(messageHex string) (*bobSignedMessage, error) {
	raw, err := hex.DecodeString(messageHex)
	if err != nil {
		return nil, &ValidationError{Reason: "signed_message_decode_failed"}
	}

	if len(raw) != bobSignedMessageSize {
		return nil, &ValidationError{Reason: fmt.Sprintf("signed_message_size_invalid:%d", len(raw))}
	}

	ts := int64(binary.LittleEndian.Uint64(raw[bobTimestampOffset : bobTimestampOffset+8]))
	if ts <= 0 || ts > math.MaxInt32*2 {
		return nil, &ValidationError{Reason: "signed_message_invalid_timestamp"}
	}

	var pubKey [32]byte
	copy(pubKey[:], raw[48:80])

	msg := &bobSignedMessage{
		Type:      decodeFixedString(raw[0:4]),
		Version:   decodeFixedString(raw[4:20]),
		Alias:     decodeFixedString(raw[20:32]),
		Uptime:    int64(binary.LittleEndian.Uint64(raw[32:40])),
		Timestamp: ts,
		PubKey:    pubKey,
	}

	return msg, nil
}

func decodeFixedString(raw []byte) string {
	for i, b := range raw {
		if b == 0 {
			return string(raw[:i])
		}
	}
	return string(raw)
}

// normalizeBobAliasForCompare mirrors Bob's fixed 12-byte alias field encoding
func normalizeBobAliasForCompare(alias string) string {
	raw := []byte(alias)
	if len(raw) > bobAliasMaxBytes {
		raw = raw[:bobAliasMaxBytes]
	}
	for i, b := range raw {
		if b == 0 {
			return string(raw[:i])
		}
	}
	return string(raw)
}

// calculates sync score based on ticks behind reference
func (v *Validator) calculateSyncScore(nodeTick uint32) float64 {
	refTick := v.reference.GetTick()

	if nodeTick >= refTick {
		return 100.0
	}

	ticksBehind := int(refTick - nodeTick)

	// Within buffer -> full score
	if ticksBehind <= v.cfg.TickBuffer {
		return 100.0
	}

	// Beyond buffer -> decay score
	excess := ticksBehind - v.cfg.TickBuffer
	score := 100.0 - (float64(excess) * v.cfg.DecayRate)

	if score < 0 {
		return 0
	}

	return score
}

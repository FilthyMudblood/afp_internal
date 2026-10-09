package topology

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
)

// Identity holds the local node's DID-bound ed25519 keypair used to sign TopologyWarnings.
type Identity struct {
	DID        string
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
}

func GenerateIdentity(did string) (*Identity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &Identity{DID: did, PrivateKey: priv, PublicKey: pub}, nil
}

// PublicKeyDirectory maps peer DID → ed25519 public key for gossip verification.
type PublicKeyDirectory struct {
	mu   sync.RWMutex
	keys map[string]ed25519.PublicKey
}

func NewPublicKeyDirectory() *PublicKeyDirectory {
	return &PublicKeyDirectory{keys: make(map[string]ed25519.PublicKey)}
}

func (d *PublicKeyDirectory) Register(did string, pub ed25519.PublicKey) {
	d.mu.Lock()
	defer d.mu.Unlock()
	cp := make(ed25519.PublicKey, len(pub))
	copy(cp, pub)
	d.keys[did] = cp
}

func (d *PublicKeyDirectory) Lookup(did string) (ed25519.PublicKey, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	pub, ok := d.keys[did]
	return pub, ok
}

// WarningSigningPayload is the canonical byte layout signed by the reporter.
// Format: reporter_id \x00 isolated_peer_id \x00 epoch(uint64 BE)
func WarningSigningPayload(w *TopologyWarning) []byte {
	buf := make([]byte, 0, len(w.ReporterID)+len(w.IsolatedPeerID)+10)
	buf = append(buf, []byte(w.ReporterID)...)
	buf = append(buf, 0)
	buf = append(buf, []byte(w.IsolatedPeerID)...)
	buf = append(buf, 0)
	var epoch [8]byte
	binary.BigEndian.PutUint64(epoch[:], w.Epoch)
	buf = append(buf, epoch[:]...)
	return buf
}

func (id *Identity) SignWarning(w *TopologyWarning) error {
	if id == nil || len(id.PrivateKey) == 0 {
		return errors.New("afp: missing gossip identity")
	}
	w.ReporterID = id.DID
	w.Signature = ed25519.Sign(id.PrivateKey, WarningSigningPayload(w))
	return nil
}

func VerifyWarningSignature(w *TopologyWarning, pub ed25519.PublicKey) bool {
	if w == nil || len(w.Signature) == 0 || len(pub) != ed25519.PublicKeySize {
		return false
	}
	return ed25519.Verify(pub, WarningSigningPayload(w), w.Signature)
}

func WarningDedupKey(w *TopologyWarning) string {
	return fmt.Sprintf("%s|%s|%d", w.ReporterID, w.IsolatedPeerID, w.Epoch)
}

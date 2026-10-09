package topology

import (
	"context"
	"log/slog"
	"sync"
)

// TopologyWarning is a signed out-of-band P2P alert regarding an isolated peer.
type TopologyWarning struct {
	IsolatedPeerID string
	ReporterID     string
	Epoch          uint64
	Signature      []byte
}

// WarningDisposition is the result of handling an inbound TopologyWarning.
type WarningDisposition int

const (
	WarningAccepted WarningDisposition = iota
	WarningRejectedUnsigned
	WarningRejectedUnknownReporter
	WarningRejectedBadSignature
	WarningRejectedDuplicate
)

func (d WarningDisposition) String() string {
	switch d {
	case WarningAccepted:
		return "accepted"
	case WarningRejectedUnsigned:
		return "rejected_unsigned"
	case WarningRejectedUnknownReporter:
		return "rejected_unknown_reporter"
	case WarningRejectedBadSignature:
		return "rejected_bad_signature"
	case WarningRejectedDuplicate:
		return "rejected_duplicate"
	default:
		return "unknown"
	}
}

// NeighborStore provides thread-safe access to local topology state.
type NeighborStore interface {
	GetTrustedNeighbors(minCVPSThreshold float64) []string
	ApplyPreemptiveDecay(peerID string, decayFactor float64)
	ResolveEndpoint(peerID string) (string, bool)
	UpsertEndpoint(peerID string, endpoint string)
}

const (
	coreRelayCVPThreshold = 0.8
	preemptiveDecayFactor = 0.5
	// Unverified / forged gossip damages the reporter's local CVP (cliff penalty).
	poisonReporterDecay = 0.25
)

// GossipBroadcaster handles asynchronous, targeted propagation of isolation events.
// Inbound unsigned or invalidly signed warnings are discarded as noise at line rate.
type GossipBroadcaster struct {
	mu       sync.RWMutex
	store    NeighborStore
	identity *Identity
	keys     *PublicKeyDirectory
	seen     sync.Map // dedup: WarningDedupKey → struct{}
}

func NewGossipBroadcaster(identity *Identity, store NeighborStore, keys *PublicKeyDirectory) *GossipBroadcaster {
	if keys == nil {
		keys = NewPublicKeyDirectory()
	}
	if identity != nil {
		keys.Register(identity.DID, identity.PublicKey)
	}
	return &GossipBroadcaster{
		store:    store,
		identity: identity,
		keys:     keys,
	}
}

func (g *GossipBroadcaster) LocalDID() string {
	if g.identity == nil {
		return ""
	}
	return g.identity.DID
}

func (g *GossipBroadcaster) PublicKeys() *PublicKeyDirectory {
	return g.keys
}

// BroadcastWarning signs a TopologyWarning and fans out to core relays (CVP ≥ 0.8).
// Wire send remains best-effort / TODO; signing is mandatory before any emit.
func (g *GossipBroadcaster) BroadcastWarning(ctx context.Context, isolatedPeer string, currentEpoch uint64) {
	go func() {
		trustedPeers := g.store.GetTrustedNeighbors(coreRelayCVPThreshold)
		if len(trustedPeers) == 0 {
			return
		}

		warning := &TopologyWarning{
			IsolatedPeerID: isolatedPeer,
			Epoch:          currentEpoch,
		}
		if err := g.identity.SignWarning(warning); err != nil {
			slog.Error("refusing to broadcast unsigned topology warning", "error", err)
			return
		}

		slog.Warn("broadcasting signed topology warning to core neighbors",
			"isolated_peer", isolatedPeer,
			"reporter", warning.ReporterID,
			"trusted_targets", len(trustedPeers),
			"signature_bytes", len(warning.Signature))

		// TODO: parallel P2P transmission to trustedPeers.
		_ = ctx
		_ = warning
		_ = trustedPeers
	}()
}

// HandleIncomingWarning verifies reporter signature before applying preemptive CVP decay.
// Unverified hearsay is discarded; forged signatures cliff-penalize the claimed reporter when known.
func (g *GossipBroadcaster) HandleIncomingWarning(warning *TopologyWarning) WarningDisposition {
	if warning == nil || len(warning.Signature) == 0 {
		slog.Info("discarding unsigned topology warning as noise")
		return WarningRejectedUnsigned
	}

	key := WarningDedupKey(warning)
	if _, loaded := g.seen.LoadOrStore(key, struct{}{}); loaded {
		return WarningRejectedDuplicate
	}

	pub, ok := g.keys.Lookup(warning.ReporterID)
	if !ok {
		slog.Info("discarding topology warning from unknown reporter",
			"reporter", warning.ReporterID)
		return WarningRejectedUnknownReporter
	}

	if !VerifyWarningSignature(warning, pub) {
		slog.Warn("discarding forged topology warning; penalizing reporter CVP",
			"reporter", warning.ReporterID,
			"target", warning.IsolatedPeerID)
		g.store.ApplyPreemptiveDecay(warning.ReporterID, poisonReporterDecay)
		return WarningRejectedBadSignature
	}

	slog.Info("accepted signed topology warning, applying preemptive decay",
		"target", warning.IsolatedPeerID,
		"reporter", warning.ReporterID)
	g.store.ApplyPreemptiveDecay(warning.IsolatedPeerID, preemptiveDecayFactor)
	return WarningAccepted
}

// SignWarningForTest exposes signing for unit tests / harnesses.
func (g *GossipBroadcaster) SignWarningForTest(w *TopologyWarning) error {
	return g.identity.SignWarning(w)
}

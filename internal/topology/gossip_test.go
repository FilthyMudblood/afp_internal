package topology

import (
	"crypto/ed25519"
	"testing"
)

func newTestGossip(t *testing.T, did string) (*GossipBroadcaster, *InMemoryNeighborStore, *Identity) {
	t.Helper()
	id, err := GenerateIdentity(did)
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	store := NewInMemoryNeighborStore()
	keys := NewPublicKeyDirectory()
	g := NewGossipBroadcaster(id, store, keys)
	return g, store, id
}

func TestHandleIncomingWarningRejectsUnsigned(t *testing.T) {
	g, store, _ := newTestGossip(t, "did:afp:receiver")
	store.UpsertNeighbor("did:afp:victim", 1.0)

	got := g.HandleIncomingWarning(&TopologyWarning{
		IsolatedPeerID: "did:afp:victim",
		ReporterID:     "did:afp:attacker",
		Epoch:          7,
		Signature:      nil,
	})
	if got != WarningRejectedUnsigned {
		t.Fatalf("want unsigned reject, got %v", got)
	}
	if cvp, _ := store.GetCVP("did:afp:victim"); cvp != 1.0 {
		t.Fatalf("unsigned noise must not decay victim CVP, got %v", cvp)
	}
}

func TestHandleIncomingWarningRejectsUnknownReporter(t *testing.T) {
	g, store, _ := newTestGossip(t, "did:afp:receiver")
	store.UpsertNeighbor("did:afp:victim", 1.0)

	got := g.HandleIncomingWarning(&TopologyWarning{
		IsolatedPeerID: "did:afp:victim",
		ReporterID:     "did:afp:stranger",
		Epoch:          7,
		Signature:      []byte("not-empty-but-untrusted"),
	})
	if got != WarningRejectedUnknownReporter {
		t.Fatalf("want unknown reporter reject, got %v", got)
	}
	if cvp, _ := store.GetCVP("did:afp:victim"); cvp != 1.0 {
		t.Fatalf("unknown reporter must not decay victim, got %v", cvp)
	}
}

func TestHandleIncomingWarningRejectsBadSignatureAndPenalizesReporter(t *testing.T) {
	receiver, store, _ := newTestGossip(t, "did:afp:receiver")
	attacker, err := GenerateIdentity("did:afp:attacker")
	if err != nil {
		t.Fatal(err)
	}
	receiver.PublicKeys().Register(attacker.DID, attacker.PublicKey)
	store.UpsertNeighbor(attacker.DID, 1.0)
	store.UpsertNeighbor("did:afp:victim", 1.0)

	w := &TopologyWarning{
		IsolatedPeerID: "did:afp:victim",
		ReporterID:     attacker.DID,
		Epoch:          3,
	}
	if err := attacker.SignWarning(w); err != nil {
		t.Fatal(err)
	}
	w.IsolatedPeerID = "did:afp:someone-else"
	store.UpsertNeighbor("did:afp:someone-else", 1.0)

	got := receiver.HandleIncomingWarning(w)
	if got != WarningRejectedBadSignature {
		t.Fatalf("want bad signature reject, got %v", got)
	}
	if cvp, _ := store.GetCVP(attacker.DID); cvp != poisonReporterDecay {
		t.Fatalf("forged gossip should cliff-penalize reporter: got %v want %v", cvp, poisonReporterDecay)
	}
	if cvp, _ := store.GetCVP("did:afp:someone-else"); cvp != 1.0 {
		t.Fatalf("forged target must not be decayed, got %v", cvp)
	}
}

func TestHandleIncomingWarningAcceptsValidSignature(t *testing.T) {
	receiver, store, _ := newTestGossip(t, "did:afp:receiver")
	reporter, err := GenerateIdentity("did:afp:reporter")
	if err != nil {
		t.Fatal(err)
	}
	receiver.PublicKeys().Register(reporter.DID, reporter.PublicKey)
	store.UpsertNeighbor("did:afp:victim", 1.0)

	w := &TopologyWarning{
		IsolatedPeerID: "did:afp:victim",
		Epoch:          9,
	}
	if err := reporter.SignWarning(w); err != nil {
		t.Fatal(err)
	}

	got := receiver.HandleIncomingWarning(w)
	if got != WarningAccepted {
		t.Fatalf("want accepted, got %v", got)
	}
	if cvp, _ := store.GetCVP("did:afp:victim"); cvp != preemptiveDecayFactor {
		t.Fatalf("victim CVP want %v got %v", preemptiveDecayFactor, cvp)
	}

	got = receiver.HandleIncomingWarning(w)
	if got != WarningRejectedDuplicate {
		t.Fatalf("want duplicate reject, got %v", got)
	}
	if cvp, _ := store.GetCVP("did:afp:victim"); cvp != preemptiveDecayFactor {
		t.Fatalf("duplicate must not re-decay, got %v", cvp)
	}
}

func TestBroadcastWarningSignsBeforeEmit(t *testing.T) {
	g, store, id := newTestGossip(t, "did:afp:local")
	store.UpsertNeighbor("did:afp:core", 0.95)

	w := &TopologyWarning{IsolatedPeerID: "did:afp:bad", Epoch: 42}
	if err := g.SignWarningForTest(w); err != nil {
		t.Fatal(err)
	}
	if w.ReporterID != id.DID {
		t.Fatalf("reporter=%q want %q", w.ReporterID, id.DID)
	}
	if len(w.Signature) != ed25519.SignatureSize {
		t.Fatalf("signature size=%d want %d", len(w.Signature), ed25519.SignatureSize)
	}
	if !VerifyWarningSignature(w, id.PublicKey) {
		t.Fatal("self-signed warning failed verification")
	}
}

func TestVerifyWarningRejectsEmptyKey(t *testing.T) {
	w := &TopologyWarning{IsolatedPeerID: "x", ReporterID: "y", Epoch: 1, Signature: []byte{1, 2, 3}}
	if VerifyWarningSignature(w, nil) {
		t.Fatal("nil pubkey must not verify")
	}
}

func TestGossipNoiseTable(t *testing.T) {
	receiver, store, _ := newTestGossip(t, "did:afp:receiver")
	store.UpsertNeighbor("did:afp:victim", 1.0)

	reporter, err := GenerateIdentity("did:afp:reporter")
	if err != nil {
		t.Fatal(err)
	}
	receiver.PublicKeys().Register(reporter.DID, reporter.PublicKey)

	valid := &TopologyWarning{IsolatedPeerID: "did:afp:victim", Epoch: 1}
	if err := reporter.SignWarning(valid); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		w    *TopologyWarning
		want WarningDisposition
	}{
		{"nil", nil, WarningRejectedUnsigned},
		{"empty_sig", &TopologyWarning{IsolatedPeerID: "did:afp:victim", ReporterID: reporter.DID, Epoch: 2}, WarningRejectedUnsigned},
		{"valid", valid, WarningAccepted},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := receiver.HandleIncomingWarning(tc.w)
			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}

	if cvp, _ := store.GetCVP("did:afp:victim"); cvp != preemptiveDecayFactor {
		t.Fatalf("only valid warning should decay victim once: got %v", cvp)
	}
}

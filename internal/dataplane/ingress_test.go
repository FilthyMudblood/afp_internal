package dataplane

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FilthyMudblood/aegis-fabric/internal/config"
	afpcontrol "github.com/FilthyMudblood/aegis-fabric/internal/control"
	"github.com/FilthyMudblood/aegis-fabric/internal/core"
	"github.com/FilthyMudblood/aegis-fabric/internal/topology"
	afpv1 "github.com/FilthyMudblood/aegis-fabric/pkg/protocol/v1"
)

func newTestSEA(mode config.RunMode, simulatedUsage float64) *SingleExecutionAuthority {
	return newTestSEAWith(mode, simulatedUsage, func() uint64 { return uint64(time.Now().Unix()) }, nil)
}

func newTestSEAWith(
	mode config.RunMode,
	simulatedUsage float64,
	epochClock func() uint64,
	gossip *topology.GossipBroadcaster,
) *SingleExecutionAuthority {
	cfg := &config.SidecarConfig{
		Mode: mode,
		Core: config.CoreConfig{
			MaxToolConcurrency: 50,
			MemoryWarnRatio:    0.75,
			OOMPanicRatio:      0.90,
		},
	}
	entropy := core.NewEntropyMonitor(&cfg.Core, &core.MockMetricsProvider{SimulatedUsage: simulatedUsage})
	return NewSingleExecutionAuthority(cfg, entropy, epochClock, gossip)
}

func TestHandleIngress_RecursionBreaker(t *testing.T) {
	sea := newTestSEA(config.ModeEnterpriseMesh, 0.1)
	err := sea.HandleIngress(context.Background(), "did:test:peer", &afpv1.GovernanceHeader{
		RecursionDepth: 11,
		CvpScore:       0.9,
		EntropyLoad:    &afpv1.EntropyLoad{ResourceAsymmetryRatio: 0.1},
	})
	if err == nil {
		t.Fatal("expected recursion breaker error, got nil")
	}
	if got := err.Error(); got != "afp-core: recursion depth exceeded physical limit, network loop detected" {
		t.Fatalf("unexpected error: %v", got)
	}
}

func TestHandleIngress_OOMCircuitBreaker(t *testing.T) {
	sea := newTestSEA(config.ModeEnterpriseMesh, 0.95)
	err := sea.HandleIngress(context.Background(), "did:test:peer", &afpv1.GovernanceHeader{
		RecursionDepth: 1,
		CvpScore:       0.9,
		EntropyLoad:    &afpv1.EntropyLoad{ResourceAsymmetryRatio: 0.1},
	})
	if err == nil {
		t.Fatal("expected circuit breaker error, got nil")
	}
	if got := err.Error(); got != "afp-core: critical context explosion or tool storm detected, circuit breaker open" {
		t.Fatalf("unexpected error: %v", got)
	}
}

func TestHandleIngress_EnterpriseBypassesStrangerTax(t *testing.T) {
	sea := newTestSEA(config.ModeEnterpriseMesh, 0.1)
	err := sea.HandleIngress(context.Background(), "did:test:anonymous", &afpv1.GovernanceHeader{
		RecursionDepth: 1,
		CvpScore:       0.9,
		EntropyLoad:    &afpv1.EntropyLoad{ResourceAsymmetryRatio: 0.1},
		// no collateral, no signature hash
	})
	if err != nil {
		t.Fatalf("expected request pass in enterprise mode, got error: %v", err)
	}
}

func TestHandleIngress_OpenExchangeRejectsStrangerWithoutCollateral(t *testing.T) {
	sea := newTestSEA(config.ModeOpenExchange, 0.1)
	err := sea.HandleIngress(context.Background(), "did:test:stranger", &afpv1.GovernanceHeader{
		RecursionDepth: 1,
		CvpScore:       0.9,
		EntropyLoad:    &afpv1.EntropyLoad{ResourceAsymmetryRatio: 0.1},
	})
	if !errors.Is(err, ErrStrangerTaxFailed) {
		t.Fatalf("expected ErrStrangerTaxFailed, got %v", err)
	}
}

func TestHandleIngress_OpenExchangeRejectsInvalidSignature(t *testing.T) {
	sea := newTestSEA(config.ModeOpenExchange, 0.1)
	err := sea.HandleIngress(context.Background(), "did:test:stranger", &afpv1.GovernanceHeader{
		RecursionDepth: 1,
		CvpScore:       0.95,
		EntropyLoad:    &afpv1.EntropyLoad{ResourceAsymmetryRatio: 0.1},
		DependencyCollateral: &afpv1.DependencyCollateral{
			CollateralType: "SYS_VIRTUAL_STAKE",
			SlashThreshold: 0.9,
		},
		// TopologyConsensusHash intentionally empty => invalid sign in open mode
	})
	if !errors.Is(err, ErrTopologyIsolated) {
		t.Fatalf("expected ErrTopologyIsolated, got %v", err)
	}
}

// Probation admits only when epoch ≡ 0 (mod ProbationProbeModulo).
func TestHandleIngress_ProbationProbeModulo(t *testing.T) {
	var epoch atomic.Uint64
	sea := newTestSEAWith(config.ModeEnterpriseMesh, 0.1, epoch.Load, nil)

	peer := "did:test:probation"
	forced := afpcontrol.NewNodeStateMachine(peer, afpcontrol.StateProbationary)
	sea.fsmRegistry.Store(peer, forced)

	header := &afpv1.GovernanceHeader{
		RecursionDepth: 1,
		CvpScore:       0.9,
		EntropyLoad:    &afpv1.EntropyLoad{ResourceAsymmetryRatio: 0.1},
	}

	epoch.Store(1)
	if err := sea.HandleIngress(context.Background(), peer, header); !errors.Is(err, ErrProbationReject) {
		t.Fatalf("epoch=1: want ErrProbationReject, got %v", err)
	}

	epoch.Store(afpcontrol.ProbationProbeModulo)
	if err := sea.HandleIngress(context.Background(), peer, header); err != nil {
		t.Fatalf("epoch=%d: want probe pass, got %v", afpcontrol.ProbationProbeModulo, err)
	}
}

func TestHandleIngress_OpenExchangeBroadcastsOnFirstIsolate(t *testing.T) {
	id, err := topology.GenerateIdentity("did:afp:sea-local")
	if err != nil {
		t.Fatal(err)
	}
	store := topology.NewInMemoryNeighborStore()
	store.UpsertNeighbor("did:afp:core", 0.95)
	gossip := topology.NewGossipBroadcaster(id, store, topology.NewPublicKeyDirectory())

	var epoch atomic.Uint64
	epoch.Store(5)
	sea := newTestSEAWith(config.ModeOpenExchange, 0.1, epoch.Load, gossip)

	err = sea.HandleIngress(context.Background(), "did:test:bad-peer", &afpv1.GovernanceHeader{
		RecursionDepth: 1,
		CvpScore:       0.95,
		EntropyLoad:    &afpv1.EntropyLoad{ResourceAsymmetryRatio: 0.1},
		DependencyCollateral: &afpv1.DependencyCollateral{
			CollateralType: "SYS_VIRTUAL_STAKE",
			SlashThreshold: 0.9,
		},
	})
	if !errors.Is(err, ErrTopologyIsolated) {
		t.Fatalf("want isolate, got %v", err)
	}

	err = sea.HandleIngress(context.Background(), "did:test:bad-peer", &afpv1.GovernanceHeader{
		RecursionDepth: 1,
		CvpScore:       0.95,
		EntropyLoad:    &afpv1.EntropyLoad{ResourceAsymmetryRatio: 0.1},
		DependencyCollateral: &afpv1.DependencyCollateral{
			CollateralType: "SYS_VIRTUAL_STAKE",
			SlashThreshold: 0.9,
		},
	})
	if !errors.Is(err, ErrTopologyIsolated) {
		t.Fatalf("want continued isolate, got %v", err)
	}
}

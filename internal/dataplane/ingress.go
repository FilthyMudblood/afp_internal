package dataplane

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/FilthyMudblood/aegis-fabric/internal/config"
	afpcontrol "github.com/FilthyMudblood/aegis-fabric/internal/control"
	"github.com/FilthyMudblood/aegis-fabric/internal/core"
	"github.com/FilthyMudblood/aegis-fabric/internal/telemetry"
	"github.com/FilthyMudblood/aegis-fabric/internal/topology"
	afpv1 "github.com/FilthyMudblood/aegis-fabric/pkg/protocol/v1"
)

var (
	ErrTopologyIsolated  = errors.New("afp: topology routed to isolated drop path")
	ErrProbationReject   = errors.New("afp: probation frequency limit exceeded")
	ErrStrangerTaxFailed = errors.New("afp: stranger tax validation failed: insufficient collateral")
)

type SingleExecutionAuthority struct {
	cfg         *config.SidecarConfig
	runtime     *config.RuntimePolicy
	entropy     *core.EntropyMonitor
	fsmRegistry sync.Map
	lastCVP     sync.Map
	mathKernel  *afpcontrol.ACCMathKernel
	localEpoch  func() uint64
	gossip      *topology.GossipBroadcaster // 注入流言广播器
}

func NewSingleExecutionAuthority(
	cfg *config.SidecarConfig,
	entropy *core.EntropyMonitor,
	epochClock func() uint64,
	gossip *topology.GossipBroadcaster,
) *SingleExecutionAuthority {
	return &SingleExecutionAuthority{
		cfg:         cfg,
		runtime:     config.NewRuntimePolicy(cfg.Core),
		entropy:     entropy,
		fsmRegistry: sync.Map{},
		lastCVP:     sync.Map{},
		mathKernel:  &afpcontrol.ACCMathKernel{},
		localEpoch:  epochClock,
		gossip:      gossip,
	}
}

func (sea *SingleExecutionAuthority) RuntimePolicy() *config.RuntimePolicy {
	return sea.runtime
}

func (sea *SingleExecutionAuthority) HandleIngress(ctx context.Context, peerID string, header *afpv1.GovernanceHeader) error {
	// ==========================================
	// 1. 分布式递归断路器 (Distributed Recursion Breaker)
	// ==========================================
	if header.RecursionDepth > sea.runtime.MaxRecursionDepth() {
		return errors.New("afp-core: recursion depth exceeded physical limit, network loop detected")
	}

	// ==========================================
	// 2. AFP-Core: 物理资源控制层
	// ==========================================
	currentEpoch := sea.localEpoch()

	// ==========================================
	// AFP-Core: 物理资源控制层 (所有模式强制执行)
	// ==========================================
	// 动态采样本地真实物理熵 (覆盖请求头中的声明，防止业务侧伪造)
	realEntropy := sea.entropy.CalculateCoreEntropy()

	// K8s 保护机制：如果当前内存逼近 OOM 或 工具调用发生雪崩
	if realEntropy >= sea.entropyCircuitBreakerThreshold() {
		telemetry.IngressActionTotal.WithLabelValues("circuit_breaker_oom").Inc()
		return errors.New("afp-core: critical context explosion or tool storm detected, circuit breaker open")
	}

	// ==========================================
	// AFP-Network: 零信任治理层 (按特征开关执行)
	// ==========================================
	val, exists := sea.fsmRegistry.Load(peerID)
	if !exists {
		// 只有在开放网络模式下，才强制征收“陌生人税”和抵押物
		if sea.cfg.Mode == config.ModeOpenExchange {
			if header.DependencyCollateral == nil || header.DependencyCollateral.SlashThreshold < 0.8 {
				telemetry.IngressActionTotal.WithLabelValues("stranger_tax_reject").Inc()
				return ErrStrangerTaxFailed
			}
		}
		// 初始化状态机
		newFSM := afpcontrol.NewNodeStateMachine(peerID, afpcontrol.StatePermissive)
		if sea.cfg.Mode == config.ModeOpenExchange {
			newFSM = afpcontrol.NewNodeStateMachine(peerID, afpcontrol.StateThrottled)
		}
		actual, _ := sea.fsmRegistry.LoadOrStore(peerID, newFSM)
		val = actual
	}

	fsm := val.(*afpcontrol.NodeStateMachine)

	// 计算 FSM 指标。企业内部默认信任签名和历史 CVP，仅依赖本地物理熵驱动状态机
	metrics := &afpcontrol.NodeMetrics{
		CVPScore:       float32(header.CvpScore),
		EntropyLoad:    realEntropy, // 强制注入本地实时熵
		CurrentEpoch:   currentEpoch,
		HasValidSign:   true, // 企业内网通过 Istio mTLS 保证，直接放行
		MaliciousSpike: false,
	}

	if sea.cfg.Mode == config.ModeOpenExchange {
		// 开放模式：触发复杂的 ACC CVP 衰减演算和签名校验
		metrics.HasValidSign = len(header.TopologyConsensusHash) > 0
		metrics.CVPScore = float32(sea.mathKernel.CalculateCVPEvolution(
			float64(header.CvpScore), 1.0, 0.0, float64(realEntropy)))
	}
	telemetry.PeerCVPScore.WithLabelValues(peerID).Set(float64(metrics.CVPScore))
	if prev, ok := sea.lastCVP.Load(peerID); ok {
		prevCVP := prev.(float32)
		if metrics.CVPScore < prevCVP {
			penalty := float64(prevCVP - metrics.CVPScore)
			telemetry.CVPPenaltyEventsTotal.WithLabelValues(peerID).Inc()
			telemetry.CVPPenaltyAmount.WithLabelValues(peerID).Observe(penalty)
		}
	}
	sea.lastCVP.Store(peerID, metrics.CVPScore)

	// 状态机演进
	decision, injectedDelay := fsm.EvaluateTransition(metrics)

	switch decision {
	case afpcontrol.ActionFastPath:
		telemetry.IngressActionTotal.WithLabelValues("fast_path").Inc()
		return nil

	case afpcontrol.ActionSlowPathWithDelay:
		telemetry.IngressActionTotal.WithLabelValues("slow_path").Inc()
		telemetry.InjectedDelayDuration.WithLabelValues(peerID).Observe(float64(injectedDelay.Milliseconds()))
		select {
		case <-time.After(injectedDelay):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}

	case afpcontrol.ActionIsolateAndBroadcast:
		telemetry.IngressActionTotal.WithLabelValues("drop_isolated").Inc()
		// 仅在开放网络模式下广播流言
		if sea.cfg.Mode == config.ModeOpenExchange && sea.gossip != nil {
			sea.gossip.BroadcastWarning(ctx, peerID, currentEpoch)
		}
		return ErrTopologyIsolated

	case afpcontrol.ActionDropPacket:
		telemetry.IngressActionTotal.WithLabelValues("drop_isolated").Inc()
		return ErrTopologyIsolated

	case afpcontrol.ActionLowFrequencyProbe:
		if currentEpoch%afpcontrol.ProbationProbeModulo != 0 {
			telemetry.IngressActionTotal.WithLabelValues("drop_probation").Inc()
			return ErrProbationReject
		}
		telemetry.IngressActionTotal.WithLabelValues("low_frequency_probe_pass").Inc()
		return nil
	}

	return ErrTopologyIsolated
}

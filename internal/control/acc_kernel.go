package control

import (
	"math"
)

const (
	// System Safety Thresholds
	ESafe       = 0.4  // 安全熵压
	EWarn       = 0.75 // 高拥塞预警线
	CVPCritical = 0.3  // 拓扑信誉破产线

	// Asymmetric hysteresis (whitepaper §3.4 / §4.2): degrade fast, recover slow.
	KIsolationEpochs     uint64  = 64  // Isolated → Probationary minimum dwell
	KProbationEpochs     uint64  = 128 // Probationary → Permissive minimum dwell
	CVPPermissiveFloor   float32 = 0.8 // Full permissive egress CVP floor
	ProbationProbeModulo uint64  = 10  // Probe admission: epoch ≡ 0 (mod 10)

	// Kernel Parameters
	Alpha  DecayFactor  = 0.95 // 历史衰减因子
	Beta   RewardWeight = 0.05 // 吞吐量奖励权重
	Lambda DecayRate    = 0.01 // 时间全局衰减常数
	Kappa  RecoveryRate = 0.02 // 对数恢复速率
)

type DecayFactor float64
type RewardWeight float64
type DecayRate float64
type RecoveryRate float64

// ACCMathKernel provides stateless, lock-free mathematical operations for governance.
// Note: This global kernel logic is strictly designed for infrastructure integration
// and topology routing. It must not be repurposed for functional A/B testing.
type ACCMathKernel struct{}

// CalculateCVPEvolution implements Formula A: CVP Evolution Equation.
// CVP_new = α * CVP_old + β * Throughput_Success - γ * Destabilization_Penalty - δ * Entropy_Load
func (k *ACCMathKernel) CalculateCVPEvolution(oldCVP, throughputSuccess, destabilizationPenalty, entropyLoad float64) float64 {
	// Gamma exponentially magnifies malicious divergence
	gamma := math.Exp(destabilizationPenalty) - 1.0
	// Delta scales mildly with external entropy
	delta := entropyLoad * 0.1

	newCVP := (float64(Alpha) * oldCVP) + (float64(Beta) * throughputSuccess) - (gamma * destabilizationPenalty) - (delta * entropyLoad)
	return math.Max(0.0, math.Min(1.0, newCVP)) // Clamp strictly between [0.0, 1.0]
}

// CalculateDecay implements Formula B: Anti-Ossification Decay.
// CVP_effective = CVP_historical * e^(-λ * Δt)
func (k *ACCMathKernel) CalculateDecay(historicalCVP float64, deltaTEpoch uint64) float64 {
	if deltaTEpoch == 0 {
		return historicalCVP
	}
	decayMultiplier := math.Exp(-float64(Lambda) * float64(deltaTEpoch))
	return historicalCVP * decayMultiplier
}

// CalculateRecovery implements Formula C: Asymmetric Hysteresis Recovery.
// CVP_recovery(t) = CVP_critical + κ * log(1 + Δt_probation)
func (k *ACCMathKernel) CalculateRecovery(deltaTProbation uint64) float64 {
	if deltaTProbation == 0 {
		return CVPCritical
	}
	// Log1p computes log(1 + x) safely and accurately for small x
	recoveryBoost := float64(Kappa) * math.Log1p(float64(deltaTProbation))
	newCVP := CVPCritical + recoveryBoost
	return math.Min(1.0, newCVP) // Cap at 1.0
}

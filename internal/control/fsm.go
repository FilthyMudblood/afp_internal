package control

import (
	"sync"
	"sync/atomic"
	"time"
)

type State uint32

const (
	StatePermissive State = iota
	StateThrottled
	StateIsolated
	StateProbationary
)

type RoutingDecision int

const (
	ActionFastPath RoutingDecision = iota
	ActionSlowPathWithDelay
	ActionDropPacket
	ActionLowFrequencyProbe
	ActionIsolateAndBroadcast // First transition into Isolated — broadcast TopologyWarning
)

type NodeMetrics struct {
	CVPScore       float32
	EntropyLoad    float32
	CurrentEpoch   uint64
	HasValidSign   bool
	MaliciousSpike bool // Dead-loop / divergent retry spike
}

type NodeStateMachine struct {
	mu             sync.RWMutex
	nodeID         string
	state          State
	lastPenalty    uint64
	lastCVP        float32
	probationStart uint64
	kernel         ACCMathKernel
}

func NewNodeStateMachine(nodeID string, initialState State) *NodeStateMachine {
	return &NodeStateMachine{
		nodeID: nodeID,
		state:  initialState,
	}
}

func (fsm *NodeStateMachine) GetState() State {
	return State(atomic.LoadUint32((*uint32)(&fsm.state)))
}

func (fsm *NodeStateMachine) LastPenaltyEpoch() uint64 {
	fsm.mu.RLock()
	defer fsm.mu.RUnlock()
	return fsm.lastPenalty
}

func (fsm *NodeStateMachine) ProbationStartEpoch() uint64 {
	fsm.mu.RLock()
	defer fsm.mu.RUnlock()
	return fsm.probationStart
}

func (fsm *NodeStateMachine) EvaluateTransition(metrics *NodeMetrics) (RoutingDecision, time.Duration) {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()

	currentState := fsm.state

	// Rule 1: signature / spike / CVP floor — instantaneous degrade
	if !metrics.HasValidSign || metrics.MaliciousSpike || metrics.CVPScore < CVPCritical {
		return fsm.enforceIsolation(metrics)
	}

	switch currentState {
	case StatePermissive:
		if metrics.EntropyLoad > EWarn {
			fsm.state = StateThrottled
			fsm.lastCVP = metrics.CVPScore
			return ActionSlowPathWithDelay, fsm.calculateLatency(metrics.EntropyLoad)
		}
		fsm.lastCVP = metrics.CVPScore
		return ActionFastPath, 0

	case StateThrottled:
		if metrics.EntropyLoad < ESafe {
			fsm.state = StatePermissive
			fsm.lastCVP = metrics.CVPScore
			return ActionFastPath, 0
		}
		fsm.lastCVP = metrics.CVPScore
		return ActionSlowPathWithDelay, fsm.calculateLatency(metrics.EntropyLoad)

	case StateIsolated:
		epochDelta := metrics.CurrentEpoch - fsm.lastPenalty
		// Isolated → Probationary only after asymmetric dwell (KIsolationEpochs).
		if epochDelta >= KIsolationEpochs {
			fsm.state = StateProbationary
			fsm.probationStart = metrics.CurrentEpoch
			fsm.lastCVP = float32(fsm.kernel.CalculateRecovery(0))
			return ActionLowFrequencyProbe, 0
		}
		return ActionDropPacket, 0

	case StateProbationary:
		// Any entropy above safe band during probation → instantaneous re-isolate.
		if metrics.EntropyLoad > ESafe {
			return fsm.enforceIsolation(metrics)
		}

		deltaProbation := metrics.CurrentEpoch - fsm.probationStart
		// Formula C floor (informational / local ledger); ACC-evolved score must clear permissive floor.
		recoveryFloor := float32(fsm.kernel.CalculateRecovery(deltaProbation))
		if recoveryFloor > fsm.lastCVP {
			fsm.lastCVP = recoveryFloor
		}

		// Asymmetric recovery: long probation dwell + high CVP. Early exit is impossible.
		if deltaProbation > KProbationEpochs && metrics.CVPScore >= CVPPermissiveFloor {
			fsm.state = StatePermissive
			fsm.lastCVP = metrics.CVPScore
			return ActionFastPath, 0
		}
		return ActionLowFrequencyProbe, 0
	}

	return ActionDropPacket, 0
}

// enforceIsolation centralizes the drop-path logic.
// lastPenalty is set only when *entering* Isolated (including re-isolate from
// probation). Refreshing the clock on every subsequent drop would starve
// recovery forever under continuous traffic — dwell time must elapse.
func (fsm *NodeStateMachine) enforceIsolation(metrics *NodeMetrics) (RoutingDecision, time.Duration) {
	wasIsolated := fsm.state == StateIsolated
	entering := !wasIsolated

	fsm.state = StateIsolated
	fsm.lastCVP = 0.0
	if entering {
		fsm.lastPenalty = metrics.CurrentEpoch
		fsm.probationStart = 0
		return ActionIsolateAndBroadcast, 0
	}
	return ActionDropPacket, 0
}

func (fsm *NodeStateMachine) calculateLatency(entropy float32) time.Duration {
	if entropy <= EWarn {
		return 500 * time.Millisecond
	}
	excess := (entropy - EWarn) / (1.0 - EWarn)
	delayMs := 500 + int64(excess*1500)
	return time.Duration(delayMs) * time.Millisecond
}

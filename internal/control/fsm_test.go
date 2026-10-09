package control

import "testing"

func TestFSMFastPathFromPermissive(t *testing.T) {
	fsm := NewNodeStateMachine("n1", StatePermissive)
	decision, delay := fsm.EvaluateTransition(&NodeMetrics{
		CVPScore:       0.95,
		EntropyLoad:    0.2,
		CurrentEpoch:   10,
		HasValidSign:   true,
		MaliciousSpike: false,
	})
	if decision != ActionFastPath {
		t.Fatalf("expected fast path, got %v", decision)
	}
	if delay != 0 {
		t.Fatalf("expected zero delay, got %v", delay)
	}
	if got := fsm.GetState(); got != StatePermissive {
		t.Fatalf("expected permissive state, got %v", got)
	}
}

func TestFSMThrottleAndDelay(t *testing.T) {
	fsm := NewNodeStateMachine("n1", StatePermissive)
	decision, delay := fsm.EvaluateTransition(&NodeMetrics{
		CVPScore:       0.95,
		EntropyLoad:    0.8,
		CurrentEpoch:   10,
		HasValidSign:   true,
		MaliciousSpike: false,
	})
	if decision != ActionSlowPathWithDelay {
		t.Fatalf("expected slow path, got %v", decision)
	}
	if delay <= 0 {
		t.Fatalf("expected positive delay, got %v", delay)
	}
	if got := fsm.GetState(); got != StateThrottled {
		t.Fatalf("expected throttled state, got %v", got)
	}
}

func TestFSMFirstIsolationBroadcastThenDrop(t *testing.T) {
	fsm := NewNodeStateMachine("n1", StatePermissive)
	decision1, _ := fsm.EvaluateTransition(&NodeMetrics{
		CVPScore:       0.9,
		EntropyLoad:    0.1,
		CurrentEpoch:   10,
		HasValidSign:   false,
		MaliciousSpike: false,
	})
	if decision1 != ActionIsolateAndBroadcast {
		t.Fatalf("expected isolate+broadcast on first isolation, got %v", decision1)
	}
	if fsm.LastPenaltyEpoch() != 10 {
		t.Fatalf("expected lastPenalty=10, got %d", fsm.LastPenaltyEpoch())
	}

	decision2, _ := fsm.EvaluateTransition(&NodeMetrics{
		CVPScore:       0.9,
		EntropyLoad:    0.1,
		CurrentEpoch:   11,
		HasValidSign:   false,
		MaliciousSpike: false,
	})
	if decision2 != ActionDropPacket {
		t.Fatalf("expected drop on repeated isolation, got %v", decision2)
	}
	// Penalty clock must NOT refresh on subsequent drops (asymmetric dwell).
	if fsm.LastPenaltyEpoch() != 10 {
		t.Fatalf("lastPenalty refreshed on subsequent drop: got %d", fsm.LastPenaltyEpoch())
	}
}

func TestFSMIsolatedToProbation(t *testing.T) {
	fsm := NewNodeStateMachine("n1", StateIsolated)
	fsm.lastPenalty = 1
	decision, _ := fsm.EvaluateTransition(&NodeMetrics{
		CVPScore:       0.8,
		EntropyLoad:    0.1,
		CurrentEpoch:   1 + KIsolationEpochs,
		HasValidSign:   true,
		MaliciousSpike: false,
	})
	if decision != ActionLowFrequencyProbe {
		t.Fatalf("expected low frequency probe, got %v", decision)
	}
	if got := fsm.GetState(); got != StateProbationary {
		t.Fatalf("expected probationary state, got %v", got)
	}
}

func TestFSMIsolatedCannotEnterProbationBeforeDwell(t *testing.T) {
	fsm := NewNodeStateMachine("n1", StateIsolated)
	fsm.lastPenalty = 100
	decision, _ := fsm.EvaluateTransition(&NodeMetrics{
		CVPScore:     0.9,
		EntropyLoad:  0.1,
		CurrentEpoch: 100 + KIsolationEpochs - 1,
		HasValidSign: true,
	})
	if decision != ActionDropPacket {
		t.Fatalf("expected drop before isolation dwell, got %v", decision)
	}
	if got := fsm.GetState(); got != StateIsolated {
		t.Fatalf("expected still isolated, got %v", got)
	}
}

func TestFSMProbationRecovery(t *testing.T) {
	fsm := NewNodeStateMachine("n1", StateProbationary)
	fsm.probationStart = 1
	decision, _ := fsm.EvaluateTransition(&NodeMetrics{
		CVPScore:       0.85,
		EntropyLoad:    0.2,
		CurrentEpoch:   1 + KProbationEpochs + 1,
		HasValidSign:   true,
		MaliciousSpike: false,
	})
	if decision != ActionFastPath {
		t.Fatalf("expected fast path recovery, got %v", decision)
	}
	if got := fsm.GetState(); got != StatePermissive {
		t.Fatalf("expected permissive state, got %v", got)
	}
}

func TestFSMProbationRejectsEarlyRecovery(t *testing.T) {
	fsm := NewNodeStateMachine("n1", StateProbationary)
	fsm.probationStart = 10
	decision, _ := fsm.EvaluateTransition(&NodeMetrics{
		CVPScore:     0.99,
		EntropyLoad:  0.1,
		CurrentEpoch: 10 + KProbationEpochs, // delta == 128, need > 128
		HasValidSign: true,
	})
	if decision != ActionLowFrequencyProbe {
		t.Fatalf("expected still probing at exact KProbationEpochs, got %v", decision)
	}
	if got := fsm.GetState(); got != StateProbationary {
		t.Fatalf("expected still probationary, got %v", got)
	}
}

func TestFSMProbationRejectsLowCVPEvenAfterDwell(t *testing.T) {
	fsm := NewNodeStateMachine("n1", StateProbationary)
	fsm.probationStart = 1
	decision, _ := fsm.EvaluateTransition(&NodeMetrics{
		CVPScore:     0.79, // below CVPPermissiveFloor
		EntropyLoad:  0.1,
		CurrentEpoch: 1 + KProbationEpochs + 10,
		HasValidSign: true,
	})
	if decision != ActionLowFrequencyProbe {
		t.Fatalf("expected probe when CVP below permissive floor, got %v", decision)
	}
	if got := fsm.GetState(); got != StateProbationary {
		t.Fatalf("expected still probationary, got %v", got)
	}
}

func TestFSMProbationEntropySpikeReIsolates(t *testing.T) {
	fsm := NewNodeStateMachine("n1", StateProbationary)
	fsm.probationStart = 50
	decision, _ := fsm.EvaluateTransition(&NodeMetrics{
		CVPScore:     0.9,
		EntropyLoad:  ESafe + 0.01,
		CurrentEpoch: 60,
		HasValidSign: true,
	})
	if decision != ActionIsolateAndBroadcast {
		t.Fatalf("expected re-isolate+broadcast on probation spike, got %v", decision)
	}
	if got := fsm.GetState(); got != StateIsolated {
		t.Fatalf("expected isolated, got %v", got)
	}
	if fsm.LastPenaltyEpoch() != 60 {
		t.Fatalf("expected penalty clock reset on re-isolate, got %d", fsm.LastPenaltyEpoch())
	}
}

// TestFSMThrashingAsymmetricRecovery: degrade is instant; recovery cannot thrash
// quickly between Isolated ↔ Permissive under bursty "good then bad" behavior.
func TestFSMThrashingAsymmetricRecovery(t *testing.T) {
	fsm := NewNodeStateMachine("peer-x", StatePermissive)

	good := func(epoch uint64, cvp, entropy float32) *NodeMetrics {
		return &NodeMetrics{
			CVPScore:     cvp,
			EntropyLoad:  entropy,
			CurrentEpoch: epoch,
			HasValidSign: true,
		}
	}

	// Instant degrade on invalid attestation.
	d, _ := fsm.EvaluateTransition(&NodeMetrics{
		CVPScore: 0.9, EntropyLoad: 0.1, CurrentEpoch: 100, HasValidSign: false,
	})
	if d != ActionIsolateAndBroadcast || fsm.GetState() != StateIsolated {
		t.Fatalf("expected first isolate, got decision=%v state=%v", d, fsm.GetState())
	}
	penalty := fsm.LastPenaltyEpoch()

	// Continuous bad traffic must not refresh the dwell clock.
	for epoch := uint64(101); epoch < 100+KIsolationEpochs; epoch++ {
		d, _ = fsm.EvaluateTransition(&NodeMetrics{
			CVPScore: 0.05, EntropyLoad: 0.9, CurrentEpoch: epoch, HasValidSign: false,
		})
		if d != ActionDropPacket {
			t.Fatalf("epoch %d: expected drop while isolated, got %v", epoch, d)
		}
		if fsm.LastPenaltyEpoch() != penalty {
			t.Fatalf("epoch %d: penalty clock refreshed under continuous drop", epoch)
		}
	}

	// Still too early for probation even with good metrics one epoch short.
	d, _ = fsm.EvaluateTransition(good(penalty+KIsolationEpochs-1, 0.9, 0.1))
	if d != ActionDropPacket || fsm.GetState() != StateIsolated {
		t.Fatalf("pre-dwell: want drop/isolated, got %v/%v", d, fsm.GetState())
	}

	// Enter probation after dwell.
	d, _ = fsm.EvaluateTransition(good(penalty+KIsolationEpochs, 0.9, 0.1))
	if d != ActionLowFrequencyProbe || fsm.GetState() != StateProbationary {
		t.Fatalf("want probation, got %v/%v", d, fsm.GetState())
	}
	probationStart := fsm.ProbationStartEpoch()

	// Mid-probation: high CVP alone must not restore Permissive (no thrashing shortcut).
	d, _ = fsm.EvaluateTransition(good(probationStart+KProbationEpochs/2, 0.99, 0.1))
	if d != ActionLowFrequencyProbe || fsm.GetState() != StateProbationary {
		t.Fatalf("mid-probation shortcut forbidden: got %v/%v", d, fsm.GetState())
	}

	// Burst during probation → instantaneous re-isolate; recovery clock restarts.
	d, _ = fsm.EvaluateTransition(good(probationStart+10, 0.99, ESafe+0.2))
	if d != ActionIsolateAndBroadcast || fsm.GetState() != StateIsolated {
		t.Fatalf("want re-isolate on thrash burst, got %v/%v", d, fsm.GetState())
	}
	newPenalty := fsm.LastPenaltyEpoch()
	if newPenalty != probationStart+10 {
		t.Fatalf("re-isolate penalty epoch=%d want %d", newPenalty, probationStart+10)
	}

	// Immediately after re-isolate, even perfect metrics stay dropped.
	d, _ = fsm.EvaluateTransition(good(newPenalty+1, 0.99, 0.05))
	if d != ActionDropPacket || fsm.GetState() != StateIsolated {
		t.Fatalf("post-thrash: want drop/isolated, got %v/%v", d, fsm.GetState())
	}

	// Full clean path: dwell → probation → long clean probation → permissive.
	d, _ = fsm.EvaluateTransition(good(newPenalty+KIsolationEpochs, 0.9, 0.1))
	if fsm.GetState() != StateProbationary {
		t.Fatalf("second probation entry failed: decision=%v state=%v", d, fsm.GetState())
	}
	ps := fsm.ProbationStartEpoch()
	d, _ = fsm.EvaluateTransition(good(ps+KProbationEpochs+1, 0.85, 0.1))
	if d != ActionFastPath || fsm.GetState() != StatePermissive {
		t.Fatalf("clean recovery failed: decision=%v state=%v", d, fsm.GetState())
	}
}

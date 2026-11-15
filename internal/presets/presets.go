// Package presets holds the IR-level preemption policy presets. Values mirror the
// live GPU Booking cluster on ibm-tmm (verified 2026-11-13) and the KubeCon
// Fair Sharing deck (slides 30-48). The emitter maps presets onto versioned typed
// objects; the parse/cluster importers use them for recognition.
package presets

import "fmt"

// Preset is a named preemption/fungibility profile for a ClusterQueue.
type Preset struct {
	Key              string `json:"key"`
	Label            string `json:"label"`
	Description      string `json:"description"`
	QueueingStrategy string `json:"queueingStrategy,omitempty"` // "" = API default

	// nil field = do not emit (API server defaults apply).
	Preemption        *PreemptionSpec  `json:"preemption,omitempty"`
	FlavorFungibility *FungibilitySpec `json:"flavorFungibility,omitempty"`
}

// PreemptionSpec mirrors ClusterQueuePreemption at IR level.
type PreemptionSpec struct {
	ReclaimWithinCohort string `json:"reclaimWithinCohort,omitempty"` // Never|LowerPriority|LowerOrNewerEqualPriority|Any
	WithinClusterQueue  string `json:"withinClusterQueue,omitempty"`  // Never|LowerPriority|LowerOrNewerEqualPriority

	BorrowPolicy          string `json:"borrowPolicy,omitempty"`          // Never|LowerPriority
	BorrowMaxPriorityThrh int32  `json:"borrowMaxPriorityThreshold,omitempty"` // 0 = unset
}

// FungibilitySpec mirrors FlavorFungibility at IR level.
type FungibilitySpec struct {
	WhenCanBorrow  string `json:"whenCanBorrow,omitempty"`  // MayStopSearch|TryNextFlavor
	WhenCanPreempt string `json:"whenCanPreempt,omitempty"` // MayStopSearch|TryNextFlavor
}

// BookedTeam = the GPU Booking "user-*" reserved ClusterQueue (live-verified):
// can reclaim its reservation from any borrower, never preempts inside its own
// queue, never borrows beyond its reservation.
var BookedTeam = Preset{
	Key:   "booked-team",
	Label: "Reserved team (GPU Booking)",
	Description: "reclaimWithinCohort: Any, borrowWithinCohort: Never, withinClusterQueue: Never. " +
		"Carves a protected reservation out of the shared cohort pool; preempts borrowers when reclaiming.",
	QueueingStrategy: "BestEffortFIFO",
	Preemption: &PreemptionSpec{
		ReclaimWithinCohort: "Any",
		WithinClusterQueue:  "Never",
		BorrowPolicy:        "Never",
	},
	FlavorFungibility: &FungibilitySpec{
		WhenCanBorrow:  "MayStopSearch",
		WhenCanPreempt: "TryNextFlavor",
	},
}

// Unreserved = the GPU Booking pure-borrower queue: no preemption rights at all,
// competes for the shared pool only.
var Unreserved = Preset{
	Key:   "unreserved",
	Label: "Shared borrower (no preemption rights)",
	Description: "reclaimWithinCohort: Never, borrowWithinCohort: Never, withinClusterQueue: Never. " +
		"Pure borrower from the cohort pool; cannot preempt anyone.",
	QueueingStrategy: "BestEffortFIFO",
	Preemption: &PreemptionSpec{
		ReclaimWithinCohort: "Never",
		WithinClusterQueue:  "Never",
		BorrowPolicy:        "Never",
	},
	FlavorFungibility: &FungibilitySpec{
		WhenCanBorrow:  "MayStopSearch",
		WhenCanPreempt: "TryNextFlavor",
	},
}

// UnreservedPriority = the GPU Booking priority borrower: preempts only
// lower-priority borrowing workloads below the threshold.
var UnreservedPriority = Preset{
	Key:   "unreserved-priority",
	Label: "Priority borrower (LowerPriority + threshold)",
	Description: "reclaimWithinCohort: LowerPriority, borrowWithinCohort: {policy: LowerPriority, maxPriorityThreshold: 100}. " +
		"Preempts low-priority borrowing workloads only.",
	QueueingStrategy: "BestEffortFIFO",
	Preemption: &PreemptionSpec{
		ReclaimWithinCohort:   "LowerPriority",
		WithinClusterQueue:    "Never",
		BorrowPolicy:          "LowerPriority",
		BorrowMaxPriorityThrh: 100,
	},
	FlavorFungibility: &FungibilitySpec{
		WhenCanBorrow:  "MayStopSearch",
		WhenCanPreempt: "TryNextFlavor",
	},
}

// FairShare = the KubeCon slides 30-48 hierarchical fair-sharing member:
// lower-priority preemption within the queue, reclaims from the cohort, and
// expects fairSharing weights on cohorts/queues (share-value ordering).
var FairShare = Preset{
	Key:   "fair-share",
	Label: "Fair-share member (weights + reclaim)",
	Description: "withinClusterQueue: LowerPriority, reclaimWithinCohort: Any. Pairs with fairSharing " +
		"weights on cohorts; preempted by share-value ordering.",
	QueueingStrategy: "BestEffortFIFO",
	Preemption: &PreemptionSpec{
		ReclaimWithinCohort: "Any",
		WithinClusterQueue:  "LowerPriority",
	},
}

// Custom emits no preemption/fungibility block (API server defaults apply).
var Custom = Preset{
	Key:         "custom",
	Label:       "Custom (API defaults)",
	Description: "Emits no preemption or fungibility block; the Kueue API server defaults apply.",
}

// All lists the presets in picker order.
var All = []Preset{BookedTeam, Unreserved, UnreservedPriority, FairShare, Custom}

// Get returns the preset with the given key ("", "custom" → Custom).
func Get(key string) (Preset, error) {
	if key == "" {
		return Custom, nil
	}
	for _, p := range All {
		if p.Key == key {
			return p, nil
		}
	}
	return Preset{}, fmt.Errorf("unknown preset %q", key)
}

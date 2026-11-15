package emit

import (
	"fmt"

	"github.com/eformat/cohort-builder/internal/model"
)

// Version dispatchers: route neutral plans to the versioned typed constructors.
// Adding a Kueue version = new synced package + a case here.

func newCohortObject(version string, p *cohortPlan) (*Object, error) {
	switch version {
	case model.Version14:
		return newCohortObjectV0_18_1(p)
	case model.Version15:
		return newCohortObjectV0_19_0(p)
	default:
		return nil, fmt.Errorf("unsupported version %q", version)
	}
}

func newClusterQueueObject(version string, p *cqPlan) (*Object, error) {
	switch version {
	case model.Version14:
		return newClusterQueueObjectV0_18_1(p)
	case model.Version15:
		return newClusterQueueObjectV0_19_0(p)
	default:
		return nil, fmt.Errorf("unsupported version %q", version)
	}
}

func newLocalQueueObject(version, cqName, namespace, lqName string) Object {
	switch version {
	case model.Version14:
		return newLocalQueueObjectV0_18_1(cqName, namespace, lqName)
	case model.Version15:
		return newLocalQueueObjectV0_19_0(cqName, namespace, lqName)
	default:
		panic(fmt.Sprintf("unsupported version %q (validated upstream)", version))
	}
}

func newResourceFlavorObject(version string, rf *model.ResourceFlavor) Object {
	switch version {
	case model.Version14:
		return newResourceFlavorObjectV0_18_1(rf)
	case model.Version15:
		return newResourceFlavorObjectV0_19_0(rf)
	default:
		panic(fmt.Sprintf("unsupported version %q (validated upstream)", version))
	}
}

func newPriorityClassObject(version string, pc *model.PriorityClass) (*Object, error) {
	switch version {
	case model.Version14:
		return newPriorityClassObjectV0_18_1(pc)
	case model.Version15:
		return newPriorityClassObjectV0_19_0(pc)
	default:
		return nil, fmt.Errorf("unsupported version %q", version)
	}
}

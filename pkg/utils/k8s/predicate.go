package k8s

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var _ predicate.Predicate = CreateOrDeletePredicate{}

// CreateOrDeletePredicate triggers a reconcile only if an obejct was created or deleted.
// It completely ignores updates and generic events.
type CreateOrDeletePredicate struct{}

func (p CreateOrDeletePredicate) Create(e event.CreateEvent) bool {
	return true
}

func (p CreateOrDeletePredicate) Update(e event.UpdateEvent) bool {
	return false
}

func (p CreateOrDeletePredicate) Delete(e event.DeleteEvent) bool {
	return true
}

func (p CreateOrDeletePredicate) Generic(e event.GenericEvent) bool {
	return false
}

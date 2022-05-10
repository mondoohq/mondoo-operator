package k8s

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var _ predicate.Predicate = IgnoreGenericEventsPredicate{}

// CreateOrDeletePredicate will completely generic events.
type IgnoreGenericEventsPredicate struct{}

func (p IgnoreGenericEventsPredicate) Create(e event.CreateEvent) bool {
	return true
}

func (p IgnoreGenericEventsPredicate) Update(e event.UpdateEvent) bool {
	return true
}

func (p IgnoreGenericEventsPredicate) Delete(e event.DeleteEvent) bool {
	return true
}

func (p IgnoreGenericEventsPredicate) Generic(e event.GenericEvent) bool {
	return false
}

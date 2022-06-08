/*
Copyright 2022 Mondoo, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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

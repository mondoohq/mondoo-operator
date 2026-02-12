// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// FieldManager is the field manager name used for Server-Side Apply operations
const FieldManager = "mondoo-operator"

// ApplyOperation indicates what happened during an Apply call
type ApplyOperation int

const (
	// ApplyCreated means the resource was newly created
	ApplyCreated ApplyOperation = iota
	// ApplyUpdated means the resource existed and was modified
	ApplyUpdated
	// ApplyUnchanged means the resource existed and was not modified
	ApplyUnchanged
)

// ApplyOptions configures the behavior of the Apply function
type ApplyOptions struct {
	// ForceOwnership takes ownership of fields managed by other field managers.
	// This should be true during migration from client-side apply to SSA.
	ForceOwnership bool
}

// DefaultApplyOptions returns the default apply options.
// ForceOwnership is true to ensure smooth migration from client-side apply.
func DefaultApplyOptions() ApplyOptions {
	return ApplyOptions{
		ForceOwnership: true,
	}
}

// Apply creates or updates a Kubernetes resource using Server-Side Apply.
// The object must have TypeMeta set (APIVersion and Kind).
// If owner is non-nil, an owner reference will be set on the object.
// Returns an ApplyOperation indicating whether the resource was created, updated, or unchanged.
func Apply(ctx context.Context, c client.Client, obj, owner client.Object, logger logr.Logger, opts ApplyOptions) (ApplyOperation, error) {
	// Validate TypeMeta is set
	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Kind == "" || gvk.Version == "" {
		return ApplyUnchanged, fmt.Errorf("object must have TypeMeta set (APIVersion and Kind)")
	}

	// Check if the resource already exists to determine operation result
	existing := obj.DeepCopyObject().(client.Object)
	err := c.Get(ctx, client.ObjectKeyFromObject(obj), existing)
	isNew := apierrors.IsNotFound(err)
	if err != nil && !isNew {
		return ApplyUnchanged, fmt.Errorf("failed to get existing %s %s/%s: %w",
			gvk.Kind, obj.GetNamespace(), obj.GetName(), err)
	}

	// Set owner reference if provided
	if owner != nil {
		ownerGVK := owner.GetObjectKind().GroupVersionKind()

		// If TypeMeta is not set on the owner, try to get it from the scheme
		if ownerGVK.Kind == "" || ownerGVK.Version == "" {
			gvks, _, err := c.Scheme().ObjectKinds(owner)
			if err != nil || len(gvks) == 0 {
				return ApplyUnchanged, fmt.Errorf("owner must have TypeMeta set or be registered in scheme (APIVersion and Kind)")
			}
			ownerGVK = gvks[0]
		}

		// Create owner reference manually to avoid scheme dependency
		isController := true
		blockOwnerDeletion := true
		ownerRef := metav1.OwnerReference{
			APIVersion:         ownerGVK.GroupVersion().String(),
			Kind:               ownerGVK.Kind,
			Name:               owner.GetName(),
			UID:                owner.GetUID(),
			Controller:         &isController,
			BlockOwnerDeletion: &blockOwnerDeletion,
		}
		obj.SetOwnerReferences([]metav1.OwnerReference{ownerRef})
	}

	// Build patch options
	patchOpts := []client.PatchOption{
		client.FieldOwner(FieldManager),
	}
	if opts.ForceOwnership {
		patchOpts = append(patchOpts, client.ForceOwnership)
	}

	// Apply the resource
	if err := c.Patch(ctx, obj, client.Apply, patchOpts...); err != nil {
		return ApplyUnchanged, fmt.Errorf("failed to apply %s %s/%s: %w",
			gvk.Kind, obj.GetNamespace(), obj.GetName(), err)
	}

	// Determine operation result
	var op ApplyOperation
	switch {
	case isNew:
		op = ApplyCreated
		logger.Info("Created resource",
			"kind", gvk.Kind,
			"namespace", obj.GetNamespace(),
			"name", obj.GetName())
	default:
		// Detect updates by re-fetching and comparing with pre-Apply state.
		// This works reliably across both real API servers and fake clients.
		freshObj := existing.DeepCopyObject().(client.Object)
		if getErr := c.Get(ctx, client.ObjectKeyFromObject(obj), freshObj); getErr != nil {
			// If we can't re-fetch, assume updated to be safe
			op = ApplyUpdated
		} else if !equality.Semantic.DeepEqual(existing, freshObj) {
			op = ApplyUpdated
		} else {
			op = ApplyUnchanged
		}

		if op == ApplyUpdated {
			logger.V(1).Info("Updated resource",
				"kind", gvk.Kind,
				"namespace", obj.GetNamespace(),
				"name", obj.GetName())
		} else {
			logger.V(1).Info("Resource unchanged",
				"kind", gvk.Kind,
				"namespace", obj.GetNamespace(),
				"name", obj.GetName())
		}
	}

	return op, nil
}

// ApplyWithoutOwner creates or updates a Kubernetes resource using Server-Side Apply
// without setting an owner reference. Use this for cluster-scoped resources or
// resources that should not be garbage collected with the owner.
func ApplyWithoutOwner(ctx context.Context, c client.Client, obj client.Object, logger logr.Logger, opts ApplyOptions) (ApplyOperation, error) {
	return Apply(ctx, c, obj, nil, logger, opts)
}

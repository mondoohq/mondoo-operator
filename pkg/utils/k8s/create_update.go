// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package k8s

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func CreateOrUpdate(ctx context.Context, clnt client.Client, obj, owner client.Object, logger logr.Logger, mutate controllerutil.MutateFn) (controllerutil.OperationResult, error) {
	op, err := ctrl.CreateOrUpdate(ctx, clnt, obj, func() error {
		if err := mutate(); err != nil {
			return err
		}
		if err := ctrl.SetControllerReference(owner, obj, clnt.Scheme()); err != nil {
			logger.Error(err,
				fmt.Sprintf("failed to set controller reference on %s", obj.GetObjectKind().GroupVersionKind().String()),
				"key", client.ObjectKeyFromObject(obj).String(),
				"owner", client.ObjectKeyFromObject(owner).String())
			return err
		}
		return nil
	})
	if err != nil {
		logger.Error(err,
			fmt.Sprintf("error while creating/updating %s", obj.GetObjectKind().GroupVersionKind().String()),
			"key", client.ObjectKeyFromObject(obj).String())
		return op, err
	}
	if op != controllerutil.OperationResultNone {
		logger.Info(
			fmt.Sprintf("%s reconciled", obj.GetObjectKind().GroupVersionKind().String()),
			"operation", op,
			"key", client.ObjectKeyFromObject(obj).String())
	}
	return op, nil
}

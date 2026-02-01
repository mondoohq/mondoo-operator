// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package debouncer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mondoo.com/mondoo-operator/controllers/resource_monitor/scan_api_store"
	"go.mondoo.com/mondoo-operator/pkg/feature_flags"
	"go.mondoo.com/mondoo-operator/pkg/utils"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const defaultFlushTimeout = 5

var logger = log.Log.WithName("scan-api-store")

//go:generate ./../../../bin/mockgen -source=./debouncer.go -destination=./mock/debouncer_generated.go -package=mock

type Debouncer interface {
	Start(ctx context.Context, managedBy string)
	Add(res string)
}

type debouncer struct {
	isFirstFlush bool
	flushTimeout time.Duration
	resChan      chan string
	resources    map[string]struct{}
	scanApiStore scan_api_store.ScanApiStore
}

func NewDebouncer(scanApiStore scan_api_store.ScanApiStore) Debouncer {
	return &debouncer{
		isFirstFlush: true,
		flushTimeout: defaultFlushTimeout * time.Second,
		resChan:      make(chan string),
		resources:    make(map[string]struct{}),
		scanApiStore: scanApiStore,
	}
}

func (d *debouncer) Start(ctx context.Context, managedBy string) {
	for {
		select {
		case <-ctx.Done():
			return
		case res := <-d.resChan:
			d.resources[res] = struct{}{}
		case <-time.After(d.flushTimeout):
			// If this is the first flush do not trigger scan for the resources. Initially, when the operator
			// starts all current cluster resources are observed as "new". We don't want to scan the entire
			// cluster for every operator start.
			if d.isFirstFlush {
				d.resources = make(map[string]struct{})
				d.isFirstFlush = false
				continue
			}

			clients := d.scanApiStore.GetAll()

			for res := range d.resources {
				for _, c := range clients {
					fields := strings.Split(res, ":")
					if len(fields) != 3 {
						err := fmt.Errorf("unpacking resource to scan has unexpected number of fields")
						logger.Error(err, "skipping resource", "request", res)
						continue
					}
					namespace := fields[1]
					allow, err := utils.AllowNamespace(namespace, c.IncludeNamespaces, c.ExcludeNamespaces)
					if err != nil {
						logger.Error(err, "skipping resource", "request", res)
						continue
					}
					if allow {
						logger.Info("Reconciling change", "request", res, "integration-mrn", c.IntegrationMrn)
						if _, err := c.Client.ScheduleKubernetesResourceScan(ctx, c.IntegrationMrn, res, managedBy); err != nil {
							logger.Error(err, "Failed to schedule resource scan", "request", res)
						}
					}
				}
			}
			d.resources = make(map[string]struct{})
		}
	}
}

func (d *debouncer) Add(res string) {
	// If the resource monitor is disabled ignore the update
	if feature_flags.GetDisableResourceMonitor() {
		return
	}
	d.resChan <- res
}

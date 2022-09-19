/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
*/

package debouncer

import (
	"context"
	"time"

	"go.mondoo.com/mondoo-operator/controllers/resource_monitor/scan_api_store"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const defaultFlushTimeout = 5

var logger = log.Log.WithName("scan-api-store")

//go:generate ./../../../bin/mockgen -source=./debouncer.go -destination=./mock/debouncer_generated.go -package=mock

type Debouncer interface {
	Start()
	Add(res string)
}

type debouncer struct {
	ctx          context.Context
	flushTimeout time.Duration
	resChan      chan string
	resources    map[string]struct{}
	scanApiStore scan_api_store.ScanApiStore
}

func NewDebouncer(ctx context.Context, scanApiStore scan_api_store.ScanApiStore) Debouncer {
	return &debouncer{
		ctx:          ctx,
		flushTimeout: defaultFlushTimeout * time.Second,
		resChan:      make(chan string),
		resources:    make(map[string]struct{}),
		scanApiStore: scanApiStore,
	}
}

func (d *debouncer) Start() {
	for {
		select {
		case <-d.ctx.Done():
			return
		case res := <-d.resChan:
			d.resources[res] = struct{}{}
		case <-time.After(d.flushTimeout):
			clients := d.scanApiStore.GetAll()

			for res := range d.resources {
				for _, c := range clients {
					logger.Info("Reconciling change", "request", res, "url", c)
					if _, err := c.Client.ScheduleKubernetesResourceScan(d.ctx, c.IntegrationMrn, res); err != nil {
						logger.Error(err, "Failed to schedule resource scan", "request", res)
					}
				}
			}
			d.resources = make(map[string]struct{})
		}
	}
}

func (d *debouncer) Add(res string) {
	d.resChan <- res
}

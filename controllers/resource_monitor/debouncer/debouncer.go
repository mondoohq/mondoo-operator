/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
*/

package debouncer

import (
	"time"

	"go.mondoo.com/mondoo-operator/controllers/resource_monitor/scan_api_store"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var logger = log.Log.WithName("scan-api-store")

type Debouncer interface {
	Start()
	Add(res string)
}

type debouncer struct {
	resChan      chan string
	resources    map[string]struct{}
	scanApiStore scan_api_store.ScanApiStore
}

func NewDebouncer(scanApiStore scan_api_store.ScanApiStore) Debouncer {
	return &debouncer{
		resChan:      make(chan string),
		resources:    make(map[string]struct{}),
		scanApiStore: scanApiStore,
	}
}

func (d *debouncer) Start() {
	for {
		select {
		case res := <-d.resChan:
			d.resources[res] = struct{}{}
		case <-time.After(5 * time.Second):
			urls := d.scanApiStore.GetAll()

			for res := range d.resources {
				for _, u := range urls {
					logger.Info("Reconciling change", "request", res, "url", u)
				}
			}
			d.resources = make(map[string]struct{})
		}
	}
}

func (d *debouncer) Add(res string) {
	d.resChan <- res
}

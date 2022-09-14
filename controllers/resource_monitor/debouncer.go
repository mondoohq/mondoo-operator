/*
Copyright 2022 Mondoo, Inc.

This Source Code Form is subject to the terms of the Mozilla Public
License, v. 2.0. If a copy of the MPL was not distributed with this
file, You can obtain one at https://mozilla.org/MPL/2.0/.
*/

package resource_monitor

import "time"

type debouncer struct {
	resChan   chan string
	resources map[string]struct{}
}

func NewDebouncer() *debouncer {
	return &debouncer{
		resChan:   make(chan string),
		resources: make(map[string]struct{}),
	}
}

func (d *debouncer) Start() {
	for {
		select {
		case res := <-d.resChan:
			d.resources[res] = struct{}{}
		case <-time.After(5 * time.Second):
			for res := range d.resources {
				logger.Info("Reconciling change", "request", res)
			}
			d.resources = make(map[string]struct{})
		}
	}
}

func (d *debouncer) Add(res string) {
	d.resChan <- res
}

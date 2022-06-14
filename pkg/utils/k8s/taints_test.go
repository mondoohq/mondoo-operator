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
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestTaintsToTolerations(t *testing.T) {
	taints := []corev1.Taint{
		{
			Key:    "key",
			Value:  "value",
			Effect: corev1.TaintEffectNoExecute,
		},
		{
			Key:    "key2",
			Value:  "value2",
			Effect: corev1.TaintEffectNoSchedule,
		},
	}

	tolerations := TaintsToTolerations(taints)

	for i, taint := range taints {
		assert.Equal(t, taint.Key, tolerations[i].Key)
		assert.Equal(t, taint.Value, tolerations[i].Value)
		assert.Equal(t, taint.Effect, tolerations[i].Effect)
	}
}

func TestTaintToToleration(t *testing.T) {
	taint := corev1.Taint{
		Key:    "key",
		Value:  "value",
		Effect: corev1.TaintEffectNoExecute,
	}

	toleration := TaintToToleration(taint)

	assert.Equal(t, taint.Key, toleration.Key)
	assert.Equal(t, taint.Value, toleration.Value)
	assert.Equal(t, taint.Effect, toleration.Effect)
}

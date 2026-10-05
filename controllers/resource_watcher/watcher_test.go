// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package resource_watcher

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	ctrlcache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestResourceWatcherShouldWatchObjectLabels(t *testing.T) {
	selector := labels.SelectorFromSet(labels.Set{"scan": "enabled"})
	watcher := &ResourceWatcher{
		config: WatcherConfig{
			ObjectSelector: selector,
		},
	}

	assert.True(t, watcher.shouldWatchObjectLabels(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{"scan": "enabled"},
		},
	}))
	assert.False(t, watcher.shouldWatchObjectLabels(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{"scan": "disabled"},
		},
	}))
	assert.True(t, watcher.shouldWatchObjectLabels(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{"scan": "disabled"},
		},
	}))
}

func TestResourceWatcherShouldWatchNamespaceLabels(t *testing.T) {
	ctx := context.Background()
	namespaceSelector := labels.SelectorFromSet(labels.Set{"tenant": "team-a"})
	watcher := &ResourceWatcher{
		namespaceReader: fake.NewClientBuilder().WithObjects(
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "team-a-prod",
					Labels: map[string]string{"tenant": "team-a"},
				},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "team-b-prod",
					Labels: map[string]string{"tenant": "team-b"},
				},
			},
		).Build(),
		config: WatcherConfig{
			NamespaceSelector: namespaceSelector,
		},
	}

	assert.True(t, watcher.shouldWatchNamespaceLabels(ctx, "team-a-prod"))
	assert.False(t, watcher.shouldWatchNamespaceLabels(ctx, "team-b-prod"))
	assert.False(t, watcher.shouldWatchNamespaceLabels(ctx, "missing"))
}

func TestResourceWatcherShouldWatchNamespaceLabelsWithNegativeSelector(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name     string
		selector string
		labels   map[string]string
		want     bool
	}{
		{
			name:     "not in excludes matching namespace",
			selector: "tenant notin (team-a)",
			labels:   map[string]string{"tenant": "team-a"},
			want:     false,
		},
		{
			name:     "not in includes different namespace",
			selector: "tenant notin (team-a)",
			labels:   map[string]string{"tenant": "team-b"},
			want:     true,
		},
		{
			name:     "does not exist excludes namespace with key",
			selector: "!scan.mondoo.com/disabled",
			labels:   map[string]string{"scan.mondoo.com/disabled": "true"},
			want:     false,
		},
		{
			name:     "does not exist includes namespace without key",
			selector: "!scan.mondoo.com/disabled",
			labels:   map[string]string{"tenant": "team-a"},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			namespaceSelector, err := labels.Parse(tt.selector)
			require.NoError(t, err)
			watcher := &ResourceWatcher{
				namespaceReader: fake.NewClientBuilder().WithObjects(&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "selected",
						Labels: tt.labels,
					},
				}).Build(),
				config: WatcherConfig{
					NamespaceSelector: namespaceSelector,
				},
			}

			assert.Equal(t, tt.want, watcher.shouldWatchNamespaceLabels(ctx, "selected"))
		})
	}
}

func TestResourceWatcherShouldWatchNamespaceResource(t *testing.T) {
	namespaceSelector := labels.SelectorFromSet(labels.Set{"tenant": "team-a"})
	watcher := &ResourceWatcher{
		config: WatcherConfig{
			NamespaceSelector: namespaceSelector,
		},
	}

	assert.True(t, watcher.shouldWatchNamespaceResource(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{"tenant": "team-a"},
		},
	}))
	assert.False(t, watcher.shouldWatchNamespaceResource(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{"tenant": "team-b"},
		},
	}))
	assert.True(t, watcher.shouldWatchNamespaceResource(&corev1.Pod{}))
}

func TestResourceWatcherSelectorsDefaultToAll(t *testing.T) {
	watcher := &ResourceWatcher{}

	assert.True(t, watcher.shouldWatchObjectLabels(&corev1.Pod{}))
	assert.True(t, watcher.shouldWatchNamespaceResource(&corev1.Namespace{}))
}

// failingInformerCache is a cache whose GetInformer always fails. Other methods are
// unimplemented and panic if called.
type failingInformerCache struct {
	ctrlcache.Cache
}

func (c *failingInformerCache) GetInformer(context.Context, client.Object, ...ctrlcache.InformerGetOption) (ctrlcache.Informer, error) {
	return nil, errors.New("namespaces is forbidden")
}

func TestResourceWatcherStartFailsWithoutNamespaceInformer(t *testing.T) {
	watcher := NewResourceWatcher(&failingInformerCache{}, nil, WatcherConfig{
		NamespaceSelector: labels.SelectorFromSet(labels.Set{"tenant": "team-a"}),
	})

	err := watcher.Start(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "namespaces is forbidden")
}

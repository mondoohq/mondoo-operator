// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package resource_watcher

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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

func TestResourceWatcherShouldWatchNamespaceLabelsUsesCache(t *testing.T) {
	ctx := context.Background()
	namespaceSelector := labels.SelectorFromSet(labels.Set{"tenant": "team-a"})
	reader := fake.NewClientBuilder().WithObjects(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "cached",
			Labels: map[string]string{"tenant": "team-a"},
		},
	}).Build()
	watcher := &ResourceWatcher{
		namespaceReader:        reader,
		namespaceLabelCacheTTL: time.Minute,
		config: WatcherConfig{
			NamespaceSelector: namespaceSelector,
		},
	}

	assert.True(t, watcher.shouldWatchNamespaceLabels(ctx, "cached"))

	updated := &corev1.Namespace{}
	require.NoError(t, reader.Get(ctx, client.ObjectKey{Name: "cached"}, updated))
	updated.Labels = map[string]string{"tenant": "team-b"}
	require.NoError(t, reader.Update(ctx, updated))

	assert.True(t, watcher.shouldWatchNamespaceLabels(ctx, "cached"))
}

func TestResourceWatcherShouldWatchNamespaceLabelsRefreshesExpiredCache(t *testing.T) {
	ctx := context.Background()
	namespaceSelector := labels.SelectorFromSet(labels.Set{"tenant": "team-a"})
	reader := fake.NewClientBuilder().WithObjects(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "cached",
			Labels: map[string]string{"tenant": "team-a"},
		},
	}).Build()
	watcher := &ResourceWatcher{
		namespaceReader:        reader,
		namespaceLabelCacheTTL: time.Minute,
		config: WatcherConfig{
			NamespaceSelector: namespaceSelector,
		},
	}

	assert.True(t, watcher.shouldWatchNamespaceLabels(ctx, "cached"))

	updated := &corev1.Namespace{}
	require.NoError(t, reader.Get(ctx, client.ObjectKey{Name: "cached"}, updated))
	updated.Labels = map[string]string{"tenant": "team-b"}
	require.NoError(t, reader.Update(ctx, updated))

	watcher.namespaceLabelCacheMu.Lock()
	entry := watcher.namespaceLabelCache["cached"]
	entry.expiresAt = time.Now().Add(-time.Second)
	watcher.namespaceLabelCache["cached"] = entry
	watcher.namespaceLabelCacheMu.Unlock()

	assert.False(t, watcher.shouldWatchNamespaceLabels(ctx, "cached"))
}

func TestResourceWatcherNamespaceLabelCacheUpdatesFromInformerEvents(t *testing.T) {
	ctx := context.Background()
	namespaceSelector := labels.SelectorFromSet(labels.Set{"tenant": "team-a"})
	watcher := &ResourceWatcher{
		config: WatcherConfig{
			NamespaceSelector: namespaceSelector,
		},
	}
	handler := &namespaceLabelEventHandler{watcher: watcher}

	handler.OnAdd(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "tenant-ns",
			Labels: map[string]string{"tenant": "team-a"},
		},
	}, true)
	assert.True(t, watcher.shouldWatchNamespaceLabels(ctx, "tenant-ns"))

	handler.OnUpdate(nil, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "tenant-ns",
			Labels: map[string]string{"tenant": "team-b"},
		},
	})
	assert.False(t, watcher.shouldWatchNamespaceLabels(ctx, "tenant-ns"))

	handler.OnDelete(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "tenant-ns"},
	})
	assert.False(t, watcher.shouldWatchNamespaceLabels(ctx, "tenant-ns"))
}

func TestResourceWatcherNamespaceLabelCachePrunesExpiredEntries(t *testing.T) {
	watcher := &ResourceWatcher{
		namespaceLabelCache: map[string]namespaceLabelCacheEntry{
			"expired": {
				labels:    labels.Set{"tenant": "team-a"},
				found:     true,
				expiresAt: time.Now().Add(-time.Minute),
			},
		},
		namespaceLabelCacheTTL: time.Minute,
	}

	watcher.setCachedNamespaceLabels("fresh", labels.Set{"tenant": "team-b"}, true)

	_, ok := watcher.namespaceLabelCache["expired"]
	assert.False(t, ok)
	_, ok = watcher.namespaceLabelCache["fresh"]
	assert.True(t, ok)
}

func TestResourceWatcherNamespaceLabelCacheEvictsOldestEntryAtLimit(t *testing.T) {
	watcher := &ResourceWatcher{
		namespaceLabelCache: map[string]namespaceLabelCacheEntry{
			"oldest": {
				labels:    labels.Set{"tenant": "team-a"},
				found:     true,
				expiresAt: time.Now().Add(time.Minute),
			},
			"newest": {
				labels:    labels.Set{"tenant": "team-b"},
				found:     true,
				expiresAt: time.Now().Add(2 * time.Minute),
			},
		},
		namespaceLabelCacheTTL:        time.Minute,
		namespaceLabelCacheMaxEntries: 2,
	}

	watcher.setCachedNamespaceLabels("fresh", labels.Set{"tenant": "team-c"}, true)

	_, ok := watcher.namespaceLabelCache["oldest"]
	assert.False(t, ok)
	_, ok = watcher.namespaceLabelCache["newest"]
	assert.True(t, ok)
	_, ok = watcher.namespaceLabelCache["fresh"]
	assert.True(t, ok)
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

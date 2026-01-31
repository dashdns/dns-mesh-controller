/*
Copyright 2025.

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

package controller

import (
	"sort"
	"sync"

	"k8s.io/apimachinery/pkg/types"
)

// BlocklistEntry represents a single IP address and its blocked domains.
type BlocklistEntry struct {
	// IP is the pod's IP address
	IP string `json:"ip"`
	// Domains is the list of blocked domain patterns for this IP
	Domains []string `json:"domains"`
}

// BlocklistCache maintains the computed IP-to-domains mapping.
// It provides incremental updates when pods or policies change.
type BlocklistCache struct {
	mu sync.RWMutex

	// podIndex provides access to pod information
	podIndex *PodIndex

	// policyIndex provides access to DNS policies
	policyIndex *PolicyIndex

	// entries maps IP address to blocklist entry
	entries map[string]*BlocklistEntry
}

// NewBlocklistCache creates a new blocklist cache.
func NewBlocklistCache(podIndex *PodIndex, policyIndex *PolicyIndex) *BlocklistCache {
	return &BlocklistCache{
		podIndex:    podIndex,
		policyIndex: policyIndex,
		entries:     make(map[string]*BlocklistEntry),
	}
}

// RecalculatePod recalculates the blocklist entry for a single pod.
// This is called when a pod is created or its labels/IP change.
func (bc *BlocklistCache) RecalculatePod(namespacedName types.NamespacedName) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	pod := bc.podIndex.Get(namespacedName)
	if pod == nil {
		return
	}

	bc.recalculatePodInternal(pod)
}

// recalculatePodInternal does the actual recalculation without locking.
// Caller must hold bc.mu.
func (bc *BlocklistCache) recalculatePodInternal(pod *PodInfo) {
	if pod.IP == "" {
		return
	}

	// Collect all domains from matching policies
	domainsSet := make(map[string]struct{})
	policies := bc.policyIndex.GetAll()

	for _, policy := range policies {
		if len(policy.Spec.TargetSelector) == 0 {
			continue
		}

		if LabelsContainAll(pod.Labels, policy.Spec.TargetSelector) {
			for _, domain := range policy.Spec.BlockList {
				domainsSet[domain] = struct{}{}
			}
		}
	}

	// Convert set to sorted slice
	domains := make([]string, 0, len(domainsSet))
	for domain := range domainsSet {
		domains = append(domains, domain)
	}
	sort.Strings(domains)

	// Update or create entry
	bc.entries[pod.IP] = &BlocklistEntry{
		IP:      pod.IP,
		Domains: domains,
	}
}

// RemovePod removes a pod's blocklist entry.
// This is called when a pod is deleted.
func (bc *BlocklistCache) RemovePod(ip string) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	delete(bc.entries, ip)
}

// RecalculateForPolicy recalculates blocklist entries for all pods matching a policy's selector.
// This is called when a policy is created or updated.
func (bc *BlocklistCache) RecalculateForPolicy(selector map[string]string) {
	if len(selector) == 0 {
		return
	}

	bc.mu.Lock()
	defer bc.mu.Unlock()

	// Find all pods matching this selector and recalculate them
	matchingPods := bc.podIndex.GetPodsMatchingSelector(selector)
	for _, pod := range matchingPods {
		bc.recalculatePodInternal(pod)
	}
}

// RecalculateForPolicyChange handles policy updates where the selector may have changed.
// It recalculates pods matching both the old and new selectors.
func (bc *BlocklistCache) RecalculateForPolicyChange(oldSelector, newSelector map[string]string) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	// Collect all affected pods (matching either old or new selector)
	affectedPods := make(map[string]*PodInfo)

	if len(oldSelector) > 0 {
		for _, pod := range bc.podIndex.GetPodsMatchingSelector(oldSelector) {
			affectedPods[pod.IP] = pod
		}
	}

	if len(newSelector) > 0 {
		for _, pod := range bc.podIndex.GetPodsMatchingSelector(newSelector) {
			affectedPods[pod.IP] = pod
		}
	}

	// Recalculate all affected pods
	for _, pod := range affectedPods {
		bc.recalculatePodInternal(pod)
	}
}

// RecalculateAll recalculates blocklist entries for all pods.
// This is useful for initial sync or full refresh.
func (bc *BlocklistCache) RecalculateAll() {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	// Clear existing entries
	bc.entries = make(map[string]*BlocklistEntry)

	// Recalculate for all pods
	pods := bc.podIndex.GetAll()
	for _, pod := range pods {
		bc.recalculatePodInternal(pod)
	}
}

// GetBlocklist returns the full blocklist as a slice of entries.
// Only includes entries with non-empty domain lists.
func (bc *BlocklistCache) GetBlocklist() []BlocklistEntry {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	result := make([]BlocklistEntry, 0, len(bc.entries))
	for _, entry := range bc.entries {
		if len(entry.Domains) > 0 {
			// Make a copy of the entry
			entryCopy := BlocklistEntry{
				IP:      entry.IP,
				Domains: make([]string, len(entry.Domains)),
			}
			copy(entryCopy.Domains, entry.Domains)
			result = append(result, entryCopy)
		}
	}

	// Sort by IP for consistent output
	sort.Slice(result, func(i, j int) bool {
		return result[i].IP < result[j].IP
	})

	return result
}

// Size returns the number of entries in the cache.
func (bc *BlocklistCache) Size() int {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return len(bc.entries)
}

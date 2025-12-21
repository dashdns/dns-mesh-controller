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
	"sync"

	dnspolicyv1alpha1 "github.com/WoodProgrammer/dns-mesh-controller/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
)

// PolicyIndex maintains an in-memory index of DNS policies by their selector hash.
// This allows efficient O(1) lookups for clients querying by hash.
type PolicyIndex struct {
	mu sync.RWMutex

	// hashToPolicy maps selector hash to the policy
	// Single policy per hash as per requirements
	hashToPolicy map[string]*dnspolicyv1alpha1.DnsPolicy

	// nameToHash maps policy namespaced name to its selector hash
	// Used for reverse lookups during updates/deletes
	nameToHash map[types.NamespacedName]string
}

// NewPolicyIndex creates a new empty policy index.
func NewPolicyIndex() *PolicyIndex {
	return &PolicyIndex{
		hashToPolicy: make(map[string]*dnspolicyv1alpha1.DnsPolicy),
		nameToHash:   make(map[types.NamespacedName]string),
	}
}

// Upsert adds or updates a policy in the index.
// If the selector hash changed, it removes the old entry and adds the new one.
func (pi *PolicyIndex) Upsert(policy *dnspolicyv1alpha1.DnsPolicy, selectorHash string) {
	pi.mu.Lock()
	defer pi.mu.Unlock()

	namespacedName := types.NamespacedName{
		Namespace: policy.Namespace,
		Name:      policy.Name,
	}

	// Check if this policy was previously indexed with a different hash
	if oldHash, exists := pi.nameToHash[namespacedName]; exists && oldHash != selectorHash {
		// Remove old hash entry
		delete(pi.hashToPolicy, oldHash)
	}

	// Add/update the policy
	pi.hashToPolicy[selectorHash] = policy.DeepCopy()
	pi.nameToHash[namespacedName] = selectorHash
}

// Delete removes a policy from the index.
func (pi *PolicyIndex) Delete(namespacedName types.NamespacedName) {
	pi.mu.Lock()
	defer pi.mu.Unlock()

	// Find the hash for this policy
	if hash, exists := pi.nameToHash[namespacedName]; exists {
		// Remove from both maps
		delete(pi.hashToPolicy, hash)
		delete(pi.nameToHash, namespacedName)
	}
}

// Get retrieves a policy by its selector hash.
// Returns nil if no policy matches the hash.
func (pi *PolicyIndex) Get(selectorHash string) *dnspolicyv1alpha1.DnsPolicy {
	pi.mu.RLock()
	defer pi.mu.RUnlock()

	if policy, exists := pi.hashToPolicy[selectorHash]; exists {
		return policy.DeepCopy()
	}
	return nil
}

// GetAll returns all indexed policies.
func (pi *PolicyIndex) GetAll() []*dnspolicyv1alpha1.DnsPolicy {
	pi.mu.RLock()
	defer pi.mu.RUnlock()

	policies := make([]*dnspolicyv1alpha1.DnsPolicy, 0, len(pi.hashToPolicy))
	for _, policy := range pi.hashToPolicy {
		policies = append(policies, policy.DeepCopy())
	}
	return policies
}

// Size returns the number of policies in the index.
func (pi *PolicyIndex) Size() int {
	pi.mu.RLock()
	defer pi.mu.RUnlock()
	return len(pi.hashToPolicy)
}

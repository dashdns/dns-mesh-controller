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

// PolicyIndex maintains an in-memory index of DNS policies.
// Policies are stored by their namespaced name for efficient lookup and iteration.
type PolicyIndex struct {
	mu sync.RWMutex

	// policies maps namespaced policy name to the policy
	policies map[types.NamespacedName]*dnspolicyv1alpha1.DnsPolicy
}

// NewPolicyIndex creates a new empty policy index.
func NewPolicyIndex() *PolicyIndex {
	return &PolicyIndex{
		policies: make(map[types.NamespacedName]*dnspolicyv1alpha1.DnsPolicy),
	}
}

// Upsert adds or updates a policy in the index.
// Returns the old targetSelector if the policy existed and selector changed, nil otherwise.
func (pi *PolicyIndex) Upsert(policy *dnspolicyv1alpha1.DnsPolicy) map[string]string {
	pi.mu.Lock()
	defer pi.mu.Unlock()

	namespacedName := types.NamespacedName{
		Namespace: policy.Namespace,
		Name:      policy.Name,
	}

	var oldSelector map[string]string

	// Check if policy exists and get old selector
	if existing, exists := pi.policies[namespacedName]; exists {
		oldSelector = existing.Spec.TargetSelector
	}

	// Store a deep copy of the policy
	pi.policies[namespacedName] = policy.DeepCopy()

	return oldSelector
}

// Delete removes a policy from the index.
// Returns the policy's targetSelector if it existed, nil otherwise.
func (pi *PolicyIndex) Delete(namespacedName types.NamespacedName) map[string]string {
	pi.mu.Lock()
	defer pi.mu.Unlock()

	if existing, exists := pi.policies[namespacedName]; exists {
		selector := existing.Spec.TargetSelector
		delete(pi.policies, namespacedName)
		return selector
	}
	return nil
}

// Get retrieves a policy by its namespaced name.
// Returns nil if no policy matches.
func (pi *PolicyIndex) Get(namespacedName types.NamespacedName) *dnspolicyv1alpha1.DnsPolicy {
	pi.mu.RLock()
	defer pi.mu.RUnlock()

	if policy, exists := pi.policies[namespacedName]; exists {
		return policy.DeepCopy()
	}
	return nil
}

// GetAll returns all indexed policies.
func (pi *PolicyIndex) GetAll() []*dnspolicyv1alpha1.DnsPolicy {
	pi.mu.RLock()
	defer pi.mu.RUnlock()

	policies := make([]*dnspolicyv1alpha1.DnsPolicy, 0, len(pi.policies))
	for _, policy := range pi.policies {
		policies = append(policies, policy.DeepCopy())
	}
	return policies
}

// Size returns the number of policies in the index.
func (pi *PolicyIndex) Size() int {
	pi.mu.RLock()
	defer pi.mu.RUnlock()
	return len(pi.policies)
}

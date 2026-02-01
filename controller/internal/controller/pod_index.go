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

	"k8s.io/apimachinery/pkg/types"
)

// PodInfo represents essential pod information for blocklist matching.
type PodInfo struct {
	// IP is the pod's IP address (status.podIP)
	IP string
	// Labels are the pod's labels used for policy matching
	Labels map[string]string
	// Namespace is the pod's namespace
	Namespace string
	// Name is the pod's name
	Name string
}

// PodIndex maintains an in-memory index of running pods with their IPs and labels.
// This enables efficient label-based matching between pods and DNS policies.
type PodIndex struct {
	mu sync.RWMutex

	// pods maps namespaced pod name to pod info
	pods map[types.NamespacedName]*PodInfo

	// ipToPod maps IP address to namespaced pod name for reverse lookups
	ipToPod map[string]types.NamespacedName
}

// NewPodIndex creates a new empty pod index.
func NewPodIndex() *PodIndex {
	return &PodIndex{
		pods:    make(map[types.NamespacedName]*PodInfo),
		ipToPod: make(map[string]types.NamespacedName),
	}
}

// Upsert adds or updates a pod in the index.
// If the pod's IP changed, it updates the reverse lookup map accordingly.
func (pi *PodIndex) Upsert(namespacedName types.NamespacedName, info *PodInfo) {
	pi.mu.Lock()
	defer pi.mu.Unlock()

	// Check if pod exists with a different IP
	if existing, exists := pi.pods[namespacedName]; exists {
		if existing.IP != info.IP {
			// Remove old IP mapping
			delete(pi.ipToPod, existing.IP)
		}
	}

	// Store pod info (make a copy of labels to avoid mutation issues)
	podCopy := &PodInfo{
		IP:        info.IP,
		Labels:    make(map[string]string, len(info.Labels)),
		Namespace: info.Namespace,
		Name:      info.Name,
	}
	for k, v := range info.Labels {
		podCopy.Labels[k] = v
	}

	pi.pods[namespacedName] = podCopy
	pi.ipToPod[info.IP] = namespacedName
}

// Delete removes a pod from the index.
func (pi *PodIndex) Delete(namespacedName types.NamespacedName) {
	pi.mu.Lock()
	defer pi.mu.Unlock()

	if existing, exists := pi.pods[namespacedName]; exists {
		delete(pi.ipToPod, existing.IP)
		delete(pi.pods, namespacedName)
	}
}

// Get retrieves a pod by its namespaced name.
// Returns nil if the pod is not found.
func (pi *PodIndex) Get(namespacedName types.NamespacedName) *PodInfo {
	pi.mu.RLock()
	defer pi.mu.RUnlock()

	if info, exists := pi.pods[namespacedName]; exists {
		// Return a copy to prevent mutation
		podCopy := &PodInfo{
			IP:        info.IP,
			Labels:    make(map[string]string, len(info.Labels)),
			Namespace: info.Namespace,
			Name:      info.Name,
		}
		for k, v := range info.Labels {
			podCopy.Labels[k] = v
		}
		return podCopy
	}
	return nil
}

// GetByIP retrieves a pod by its IP address.
// Returns nil if no pod with that IP is found.
func (pi *PodIndex) GetByIP(ip string) *PodInfo {
	pi.mu.RLock()
	defer pi.mu.RUnlock()

	if namespacedName, exists := pi.ipToPod[ip]; exists {
		if info, exists := pi.pods[namespacedName]; exists {
			// Return a copy to prevent mutation
			podCopy := &PodInfo{
				IP:        info.IP,
				Labels:    make(map[string]string, len(info.Labels)),
				Namespace: info.Namespace,
				Name:      info.Name,
			}
			for k, v := range info.Labels {
				podCopy.Labels[k] = v
			}
			return podCopy
		}
	}
	return nil
}

// GetAll returns all pods in the index.
// Returns copies to prevent mutation.
func (pi *PodIndex) GetAll() []*PodInfo {
	pi.mu.RLock()
	defer pi.mu.RUnlock()

	result := make([]*PodInfo, 0, len(pi.pods))
	for _, info := range pi.pods {
		podCopy := &PodInfo{
			IP:        info.IP,
			Labels:    make(map[string]string, len(info.Labels)),
			Namespace: info.Namespace,
			Name:      info.Name,
		}
		for k, v := range info.Labels {
			podCopy.Labels[k] = v
		}
		result = append(result, podCopy)
	}
	return result
}

// GetPodsMatchingSelector returns all pods whose labels match the given selector.
// A pod matches if it has all the key-value pairs in the selector.
func (pi *PodIndex) GetPodsMatchingSelector(selector map[string]string) []*PodInfo {
	pi.mu.RLock()
	defer pi.mu.RUnlock()

	result := make([]*PodInfo, 0)
	for _, info := range pi.pods {
		if LabelsContainAll(info.Labels, selector) {
			podCopy := &PodInfo{
				IP:        info.IP,
				Labels:    make(map[string]string, len(info.Labels)),
				Namespace: info.Namespace,
				Name:      info.Name,
			}
			for k, v := range info.Labels {
				podCopy.Labels[k] = v
			}
			result = append(result, podCopy)
		}
	}
	return result
}

// Size returns the number of pods in the index.
func (pi *PodIndex) Size() int {
	pi.mu.RLock()
	defer pi.mu.RUnlock()
	return len(pi.pods)
}

// LabelsContainAll checks if the pod labels contain all the key-value pairs from the selector.
// Returns true if all selector entries are present in labels with matching values.
// Returns false if selector is empty.
func LabelsContainAll(labels, selector map[string]string) bool {
	if len(selector) == 0 {
		return false
	}

	for key, value := range selector {
		if labelValue, exists := labels[key]; !exists || labelValue != value {
			return false
		}
	}
	return true
}

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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	dnspolicyv1alpha1 "github.com/WoodProgrammer/dns-mesh-controller/api/v1alpha1"
)

// ComputeSelectorHash computes a deterministic hash of a label selector.
// The hash is computed by sorting the keys and creating a stable JSON representation.
func ComputeSelectorHash(selector map[string]string) (string, error) {
	if len(selector) == 0 {
		return "", nil
	}

	// Sort keys for deterministic output
	keys := make([]string, 0, len(selector))
	for k := range selector {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build sorted map
	sortedSelector := make(map[string]string, len(selector))
	for _, k := range keys {
		sortedSelector[k] = selector[k]
	}

	// Marshal to JSON
	data, err := json.Marshal(sortedSelector)
	if err != nil {
		return "", err
	}

	// Compute SHA256 hash
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// ComputeSpecHash computes a hash of the entire DnsPolicySpec.
// This is used to detect when the policy configuration has changed.
func ComputeSpecHash(spec *dnspolicyv1alpha1.DnsPolicySpec) (string, error) {
	// Create a normalized representation
	normalized := struct {
		TargetSelector map[string]string
		AllowList      []string
		BlockList      []string
	}{
		TargetSelector: spec.TargetSelector,
		AllowList:      spec.AllowList,
		BlockList:      spec.BlockList,
	}

	// Sort slices for deterministic output
	if normalized.AllowList != nil {
		sort.Strings(normalized.AllowList)
	}
	if normalized.BlockList != nil {
		sort.Strings(normalized.BlockList)
	}

	// Sort target selector keys
	if normalized.TargetSelector != nil {
		keys := make([]string, 0, len(normalized.TargetSelector))
		for k := range normalized.TargetSelector {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		sortedSelector := make(map[string]string, len(normalized.TargetSelector))
		for _, k := range keys {
			sortedSelector[k] = normalized.TargetSelector[k]
		}
		normalized.TargetSelector = sortedSelector
	}

	// Marshal to JSON
	data, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}

	// Compute SHA256 hash
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

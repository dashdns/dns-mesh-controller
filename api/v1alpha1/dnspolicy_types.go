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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DnsPolicySpec defines the desired state of DnsPolicy.
type DnsPolicySpec struct {
	// TargetSelector specifies the labels to match pods this policy applies to.
	// Simple key-value matching: all labels must match exactly.
	// +optional
	TargetSelector map[string]string `json:"targetSelector,omitempty"`
	// BlockList contains domain patterns that are blocked from DNS resolution.
	// +optional
	BlockList []string `json:"blockList,omitempty"`

	Subject map[string]string `json:"subject,omitempty"`
	// +optional
	DryRun bool `json:"dryrun,omitempty"`
}

// DnsPolicyStatus defines the observed state of DnsPolicy.
type DnsPolicyStatus struct {
	// SelectorHash is the hash of the TargetSelector for efficient client lookups.
	// Clients compute hash of their labels and query policies by this hash.
	// +optional
	SelectorHash string `json:"selectorHash,omitempty"`

	// SpecHash is the hash of the entire Spec for change detection.
	// Clients use this to detect if policy configuration has changed.
	// +optional
	SpecHash string `json:"specHash,omitempty"`

	// ObservedGeneration is the generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the DnsPolicy's state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// DnsPolicy is the Schema for the dnspolicies API.
type DnsPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DnsPolicySpec   `json:"spec,omitempty"`
	Status DnsPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DnsPolicyList contains a list of DnsPolicy.
type DnsPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DnsPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DnsPolicy{}, &DnsPolicyList{})
}

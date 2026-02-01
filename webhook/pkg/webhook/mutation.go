package webhook

import (
	"encoding/json"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

const (
	sidecarAnnotation = "dasdns.io/watch"
)

func shouldAdjustDnsConfig(deployment *appsv1.Deployment) bool {
	annotations := deployment.Annotations
	if annotations == nil {
		return false
	}

	value, exists := annotations[sidecarAnnotation]
	return exists && value == "true"
}

func createPatch(deployment *appsv1.Deployment, dnsServiceIP string) ([]byte, error) {
	var patches []patchOperation
	// Add DNS configuration
	dnsConfig := &corev1.PodDNSConfig{
		Nameservers: []string{dnsServiceIP},
		Searches: []string{
			"default.svc.cluster.local",
			"svc.cluster.local",
			"cluster.local",
		},
	}

	patches = append(patches, patchOperation{
		Op:    "add",
		Path:  "/spec/template/spec/dnsConfig",
		Value: dnsConfig,
	})

	patches = append(patches, patchOperation{
		Op:    "add",
		Path:  "/spec/template/spec/dnsPolicy",
		Value: corev1.DNSNone,
	})

	patchBytes, err := json.Marshal(patches)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal patches: %v", err)
	}

	return patchBytes, nil
}

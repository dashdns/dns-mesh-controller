package webhook

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

const (
	sidecarAnnotation = "sidecar-injector.io/inject"
	sidecarName       = "sidecar-dns"
)

func shouldInject(deployment *appsv1.Deployment) bool {
	annotations := deployment.Annotations
	if annotations == nil {
		return false
	}

	value, exists := annotations[sidecarAnnotation]
	return exists && value == "true"
}

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

func createPatch(deployment *appsv1.Deployment, img SidecarImage, upstreamDNSAddress string, operationalMode string) ([]byte, error) {
	var patches []patchOperation
	var envVars []corev1.EnvVar
	var hash string

	hashObject := make(map[string]string)
	controllerAddress := os.Getenv("CONTROLLER_ADDRESS")

	if len(controllerAddress) == 0 {
		controllerAddress = "http://dns-mesh-controller-controller-manager-metrics-service.dns-mesh-controller-system:5959"
	}
	hashObject = deployment.ObjectMeta.Labels
	if deployment.Spec.Template.Spec.ServiceAccountName != "" {
		hashObject = map[string]string{}
		hashObject["serviceAccount"] = deployment.Spec.Template.Spec.ServiceAccountName
	}
	hash, err := ComputeSelectorHash(hashObject)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate hash: %v", err)
	}

	if len(hash) == 0 {
		return nil, fmt.Errorf("Calculated hash is empty %v", err)
	}
	env := corev1.EnvVar{
		Name:  "DNS_MESH_CONFIG_HASH",
		Value: hash,
	}
	envVars = append(envVars, env)
	optMode := corev1.EnvVar{
		Name:  "DNS_MESH_OPERATIONAL_MODE",
		Value: operationalMode,
	}
	envVars = append(envVars, optMode)
	sidecarContainer := corev1.Container{
		Name:      sidecarName,
		Image:     fmt.Sprintf("%s:%s", img.Name, img.Tag),
		Resources: corev1.ResourceRequirements{},
		Args:      []string{"-upstream", fmt.Sprintf("%s:%s", upstreamDNSAddress, "53"), "-controller", controllerAddress},
		Env:       envVars,
	}
	// TODO : Fix possible update options in DNSConfig
	containers := deployment.Spec.Template.Spec.Containers
	if len(containers) == 0 {
		// If no containers exist, create the array with the sidecar
		patches = append(patches, patchOperation{
			Op:    "add",
			Path:  "/spec/template/spec/containers",
			Value: []corev1.Container{sidecarContainer},
		})
	} else {
		// Add sidecar to existing containers

		for _, container := range containers {
			if container.Name == "sidecar-dns" {
				return nil, nil
			}
		}
		patches = append(patches, patchOperation{
			Op:    "add",
			Path:  "/spec/template/spec/containers/-",
			Value: sidecarContainer,
		})
	}

	// Add DNS configuration
	dnsConfig := &corev1.PodDNSConfig{
		Nameservers: []string{"127.0.0.1"},
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

	// Add annotation to mark injection as completed
	if deployment.Spec.Template.Annotations == nil {
		patches = append(patches, patchOperation{
			Op:   "add",
			Path: "/spec/template/metadata/annotations",
			Value: map[string]string{
				"sidecar-injector.io/status": "injected",
			},
		})
	} else {
		patches = append(patches, patchOperation{
			Op:    "add",
			Path:  "/spec/template/metadata/annotations/sidecar-injector.io~1status",
			Value: "injected",
		})
	}

	patchBytes, err := json.Marshal(patches)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal patches: %v", err)
	}

	return patchBytes, nil
}

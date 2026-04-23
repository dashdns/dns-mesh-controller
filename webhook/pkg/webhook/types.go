package webhook

import "k8s.io/client-go/kubernetes"

const (
	HOST_NETWORK_NOT_ALLOWED = "The Spec.Template.Spec.HostNetwork cannot be true according to the DnsPolicies"
)

type patchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

type Server struct {
	k8sClient           kubernetes.Interface
	dnsServiceName      string
	dnsServiceNamespace string
}

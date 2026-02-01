package webhook

import "k8s.io/client-go/kubernetes"

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

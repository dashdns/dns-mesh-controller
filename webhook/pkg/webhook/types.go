package webhook

import "k8s.io/client-go/kubernetes"

type SidecarImage struct {
	Name string `json:"name"`
	Tag  string `json:"tag"`
}

type patchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

type Server struct {
	img                 SidecarImage
	operationalMode     string
	k8sClient           kubernetes.Interface
	dnsServiceName      string
	dnsServiceNamespace string
}

package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"slices"

	admissionv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	runtimeScheme    = runtime.NewScheme()
	codecs           = serializer.NewCodecFactory(runtimeScheme)
	deserializer     = codecs.UniversalDeserializer()
	operationalModes = []string{"balance", "strict", "flexible"}
)

func NewServer() *Server {
	sideCarImage := os.Getenv("SIDECAR_IMAGE")
	sideCarImageTag := os.Getenv("SIDECAR_IMAGE_TAG")
	operationalMode := os.Getenv("OPERATIONAL_MODE")

	if len(sideCarImage) == 0 && len(sideCarImageTag) == 0 {
		sideCarImage = "docker.io/emirozbir/sidecar-injector"
		sideCarImageTag = "latest"
	}
	img := SidecarImage{
		Name: sideCarImage,
		Tag:  sideCarImageTag,
	}

	if len(operationalMode) == 0 {
		operationalMode = "optional"
	}
	if len(operationalMode) != 0 && !slices.Contains(operationalModes, operationalMode) {
		err := errors.New("Invalid operational modes, operational modes should be in list")
		log.Fatalf("Operational modes should be in this list %s , %v", operationalModes, err)
	}
	// Get DNS service configuration from environment variables with defaults
	dnsServiceName := os.Getenv("DNS_SERVICE_NAME")
	if dnsServiceName == "" {
		dnsServiceName = "kube-dns"
	}

	dnsServiceNamespace := os.Getenv("DNS_SERVICE_NAMESPACE")
	if dnsServiceNamespace == "" {
		dnsServiceNamespace = "kube-system"
	}

	// Create Kubernetes in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("Failed to create in-cluster config: %v", err)
	}

	// Create Kubernetes clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create Kubernetes client: %v", err)
	}

	return &Server{
		img:                 img,
		operationalMode:     operationalMode,
		k8sClient:           clientset,
		dnsServiceName:      dnsServiceName,
		dnsServiceNamespace: dnsServiceNamespace,
	}
}

func (s *Server) getDNSServiceIP(ctx context.Context) (string, error) {
	// Fetch the DNS service from Kubernetes
	service, err := s.k8sClient.CoreV1().Services(s.dnsServiceNamespace).Get(ctx, s.dnsServiceName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get DNS service %s/%s: %v", s.dnsServiceNamespace, s.dnsServiceName, err)
	}

	// Check if the service has a ClusterIP
	if service.Spec.ClusterIP == "" || service.Spec.ClusterIP == corev1.ClusterIPNone {
		return "", fmt.Errorf("DNS service %s/%s does not have a valid ClusterIP", s.dnsServiceNamespace, s.dnsServiceName)
	}

	return service.Spec.ClusterIP, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body []byte
	if r.Body != nil {
		if data, err := io.ReadAll(r.Body); err == nil {
			body = data
		}
	}

	if len(body) == 0 {
		log.Println("Empty body")
		http.Error(w, "Empty body", http.StatusBadRequest)
		return
	}

	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		log.Printf("Invalid content type: %s", contentType)
		http.Error(w, "Invalid content type", http.StatusBadRequest)
		return
	}

	var admissionResponse *admissionv1.AdmissionResponse
	ar := admissionv1.AdmissionReview{}
	if _, _, err := deserializer.Decode(body, nil, &ar); err != nil {
		log.Printf("Can't decode body: %v", err)
		admissionResponse = &admissionv1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	} else {
		admissionResponse = s.mutate(&ar)
	}

	admissionReview := admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "admission.k8s.io/v1",
			Kind:       "AdmissionReview",
		},
	}
	if admissionResponse != nil {
		admissionReview.Response = admissionResponse
		if ar.Request != nil {
			admissionReview.Response.UID = ar.Request.UID
		}
	}

	resp, err := json.Marshal(admissionReview)
	if err != nil {
		log.Printf("Can't encode response: %v", err)
		http.Error(w, fmt.Sprintf("Could not encode response: %v", err), http.StatusInternalServerError)
		return
	}

	if _, err := w.Write(resp); err != nil {
		log.Printf("Can't write response: %v", err)
		http.Error(w, fmt.Sprintf("Could not write response: %v", err), http.StatusInternalServerError)
	}
}

func (s *Server) mutate(ar *admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	req := ar.Request

	log.Printf("AdmissionReview for Kind=%v, Namespace=%v Name=%v UID=%v",
		req.Kind, req.Namespace, req.Name, req.UID)

	var deployment appsv1.Deployment
	if err := json.Unmarshal(req.Object.Raw, &deployment); err != nil {
		log.Printf("Could not unmarshal raw object: %v", err)
		return &admissionv1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}
	// Check if sidecar injection is enabled via annotation
	if !shouldInject(&deployment) {
		log.Printf("Skipping injection for deployment %s/%s", deployment.Namespace, deployment.Name)
		return &admissionv1.AdmissionResponse{
			Allowed: true,
		}
	}
	// Fetch the DNS service IP
	ctx := context.Background()
	dnsServiceIP, err := s.getDNSServiceIP(ctx)
	if err != nil {
		log.Printf("Could not get DNS service IP: %v", err)
		return &admissionv1.AdmissionResponse{
			Result: &metav1.Status{
				Message: fmt.Sprintf("Failed to fetch DNS service IP: %v", err),
			},
		}
	}
	log.Printf("Using DNS service IP: %s for deployment %s/%s", dnsServiceIP, deployment.Namespace, deployment.Name)
	patchBytes := []byte{}
	if shouldInject(&deployment) {
		patchBytes, err = createPatch(&deployment, s.img, dnsServiceIP, s.operationalMode)
		if err != nil {
			log.Printf("Could not create patch: %v", err)
			return &admissionv1.AdmissionResponse{
				Result: &metav1.Status{
					Message: err.Error(),
				},
			}
		}
	}
	log.Printf("Successfully created patch for deployment %s/%s", deployment.Namespace, deployment.Name)
	return &admissionv1.AdmissionResponse{
		Allowed: true,
		Patch:   patchBytes,
		PatchType: func() *admissionv1.PatchType {
			pt := admissionv1.PatchTypeJSONPatch
			return &pt
		}(),
	}
}

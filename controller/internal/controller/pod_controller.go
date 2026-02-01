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
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// PodReconciler watches pods and updates the PodIndex and BlocklistCache.
type PodReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	PodIndex       *PodIndex
	BlocklistCache *BlocklistCache
}

// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch

// Reconcile handles pod create/update/delete events.
// It updates the PodIndex and triggers blocklist recalculation.
func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the Pod
	var pod corev1.Pod
	if err := r.Get(ctx, req.NamespacedName, &pod); err != nil {
		if apierrors.IsNotFound(err) {
			// Pod was deleted
			log.Info("Pod deleted, removing from index", "name", req.NamespacedName)

			// Get the pod info before deletion to get its IP
			existingPod := r.PodIndex.Get(req.NamespacedName)
			if existingPod != nil {
				r.BlocklistCache.RemovePod(existingPod.IP)
			}
			r.PodIndex.Delete(req.NamespacedName)

			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get Pod")
		return ctrl.Result{}, err
	}

	// Skip pods without an IP (not yet scheduled or in pending state)
	if pod.Status.PodIP == "" {
		log.V(1).Info("Pod has no IP, skipping", "name", req.NamespacedName, "phase", pod.Status.Phase)
		return ctrl.Result{}, nil
	}

	// Skip pods that are not Running
	if pod.Status.Phase != corev1.PodRunning {
		log.V(1).Info("Pod is not running, skipping", "name", req.NamespacedName, "phase", pod.Status.Phase)

		// If the pod was previously indexed but is no longer running, remove it
		existingPod := r.PodIndex.Get(req.NamespacedName)
		if existingPod != nil {
			log.Info("Pod no longer running, removing from index", "name", req.NamespacedName)
			r.BlocklistCache.RemovePod(existingPod.IP)
			r.PodIndex.Delete(req.NamespacedName)
		}

		return ctrl.Result{}, nil
	}

	// Create pod info
	podInfo := &PodInfo{
		IP:        pod.Status.PodIP,
		Labels:    pod.Labels,
		Namespace: pod.Namespace,
		Name:      pod.Name,
	}

	// Check if this is an update with changed IP
	existingPod := r.PodIndex.Get(req.NamespacedName)
	if existingPod != nil && existingPod.IP != podInfo.IP {
		// IP changed, remove old entry from blocklist cache
		r.BlocklistCache.RemovePod(existingPod.IP)
	}

	// Update pod index
	r.PodIndex.Upsert(req.NamespacedName, podInfo)
	log.Info("Pod indexed", "name", req.NamespacedName, "ip", podInfo.IP)

	// Recalculate blocklist for this pod
	r.BlocklistCache.RecalculatePod(req.NamespacedName)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		WithEventFilter(podEventFilter()).
		Named("pod").
		Complete(r)
}

// podEventFilter creates predicates to filter pod events.
// We only care about pods that have an IP and are in Running phase,
// or pods that are being deleted.
func podEventFilter() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			pod, ok := e.Object.(*corev1.Pod)
			if !ok {
				return false
			}
			// Process if pod has IP and is running
			return pod.Status.PodIP != "" && pod.Status.Phase == corev1.PodRunning
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldPod, ok := e.ObjectOld.(*corev1.Pod)
			if !ok {
				return false
			}
			newPod, ok := e.ObjectNew.(*corev1.Pod)
			if !ok {
				return false
			}

			// Process if:
			// 1. Pod became running with an IP
			// 2. Pod's IP changed
			// 3. Pod's labels changed
			// 4. Pod is no longer running (to remove from index)

			// Check if pod became running
			becameRunning := oldPod.Status.Phase != corev1.PodRunning && newPod.Status.Phase == corev1.PodRunning
			// Check if pod stopped running
			stoppedRunning := oldPod.Status.Phase == corev1.PodRunning && newPod.Status.Phase != corev1.PodRunning
			// Check if IP changed
			ipChanged := oldPod.Status.PodIP != newPod.Status.PodIP
			// Check if labels changed (simplified check)
			labelsChanged := !labelsEqual(oldPod.Labels, newPod.Labels)

			return becameRunning || stoppedRunning || ipChanged || labelsChanged
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// Always process deletions
			return true
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}
}

// labelsEqual checks if two label maps are equal.
func labelsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

// NamespacedName returns a types.NamespacedName for a pod.
func PodNamespacedName(pod *corev1.Pod) types.NamespacedName {
	return types.NamespacedName{
		Namespace: pod.Namespace,
		Name:      pod.Name,
	}
}

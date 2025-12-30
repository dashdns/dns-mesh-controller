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
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	dnsv1alpha1 "github.com/WoodProgrammer/dns-mesh-controller/api/v1alpha1"
)

const (
	dnsPolicyFinalizer = "dns.dnspolicies.io/finalizer"
)

// DnsPolicyReconciler reconciles a DnsPolicy object
type DnsPolicyReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Index    *PolicyIndex
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=dns.dnspolicies.io,resources=dnspolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dns.dnspolicies.io,resources=dnspolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dns.dnspolicies.io,resources=dnspolicies/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile reconciles a DnsPolicy object by:
// 1. Computing hashes of the targetSelector and full spec
// 2. Updating the status with computed hashes
// 3. Indexing the policy for efficient client lookups by hash
// 4. Handling deletions by removing from index
func (r *DnsPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the DnsPolicy instance
	var policy dnsv1alpha1.DnsPolicy
	if err := r.Get(ctx, req.NamespacedName, &policy); err != nil {
		if apierrors.IsNotFound(err) {
			// Policy was deleted - remove from index
			log.Info("DnsPolicy deleted, removing from index", "name", req.NamespacedName)
			r.Index.Delete(req.NamespacedName)
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get DnsPolicy")
		return ctrl.Result{}, err
	}

	// Handle deletion with finalizer
	if !policy.ObjectMeta.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&policy, dnsPolicyFinalizer) {
			// Remove from index before removing finalizer
			log.Info("DnsPolicy being deleted, removing from index", "name", req.NamespacedName)
			r.Index.Delete(req.NamespacedName)
			r.Recorder.Event(&policy, corev1.EventTypeNormal, "Deleted", "DnsPolicy removed from index")

			// Remove finalizer
			controllerutil.RemoveFinalizer(&policy, dnsPolicyFinalizer)
			if err := r.Update(ctx, &policy); err != nil {
				log.Error(err, "Failed to remove finalizer")
				r.Recorder.Event(&policy, corev1.EventTypeWarning, "FinalizerRemovalFailed", fmt.Sprintf("Failed to remove finalizer: %v", err))
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(&policy, dnsPolicyFinalizer) {
		controllerutil.AddFinalizer(&policy, dnsPolicyFinalizer)
		if err := r.Update(ctx, &policy); err != nil {
			log.Error(err, "Failed to add finalizer")
			r.Recorder.Event(&policy, corev1.EventTypeWarning, "FinalizerAddFailed", fmt.Sprintf("Failed to add finalizer: %v", err))
			return ctrl.Result{}, err
		}
		r.Recorder.Event(&policy, corev1.EventTypeNormal, "FinalizerAdded", "Finalizer added to DnsPolicy")
		return ctrl.Result{}, nil
	}

	// Validate targetSelector is not empty
	if len(policy.Spec.TargetSelector) == 0 && len(policy.Spec.Subject) == 0 {
		err := fmt.Errorf("TargetSelector or Subject cannot be empty")
		log.Error(err, "Invalid DnsPolicy spec")
		r.Recorder.Event(&policy, corev1.EventTypeWarning, "InvalidSpec", "TargetSelector or Subject cannot be empty")
		r.updateCondition(ctx, &policy, "Ready", metav1.ConditionFalse, "InvalidSpec", err.Error())
		return ctrl.Result{}, err
	}

	var hashObject map[string]string
	// Validate targetSelector is not empty
	if len(policy.Spec.TargetSelector) == 0 {
		if len(policy.Spec.Subject) == 0 {
			err := fmt.Errorf("targetSelector or Subject cannot be empty")
			log.Error(err, "Invalid DnsPolicy spec")
			r.Recorder.Event(&policy, corev1.EventTypeWarning, "InvalidSpec", "TargetSelector or Subject cannot be empty")
			r.updateCondition(ctx, &policy, "Ready", metav1.ConditionFalse, "InvalidSpec", err.Error())
			return ctrl.Result{}, err
		} else {
			hashObject = policy.Spec.Subject
		}
	} else {
		hashObject = policy.Spec.TargetSelector
	}
	// Compute selector hash
	selectorHash, err := ComputeSelectorHash(hashObject)
	if err != nil {
		log.Error(err, "Failed to compute selector hash")
		r.Recorder.Event(&policy, corev1.EventTypeWarning, "HashComputationFailed", fmt.Sprintf("Failed to compute selector hash: %v", err))
		r.updateCondition(ctx, &policy, "Ready", metav1.ConditionFalse, "HashComputationFailed", err.Error())
		return ctrl.Result{}, err
	}

	existing_index := r.Index.Get(selectorHash)

	if existing_index != nil {
		if len(existing_index.Name) != 0 && existing_index.Name != policy.Name {
			err := errors.New("DUPLICATE HASH ERROR")
			log.Error(err, fmt.Sprintf("The policy contains same hash policy with the name %s", existing_index.Name))
			r.Recorder.Event(&policy, corev1.EventTypeWarning, "DuplicatHash", fmt.Sprintf("Failed to compute spec hash: %v", err))
			r.updateCondition(ctx, &policy, "Ready", metav1.ConditionFalse, "HashComputationFailed", err.Error())
			return ctrl.Result{}, err
		}
	}
	// Compute spec hash
	specHash, err := ComputeSpecHash(&policy.Spec)
	if err != nil {
		log.Error(err, "Failed to compute spec hash")
		r.Recorder.Event(&policy, corev1.EventTypeWarning, "HashComputationFailed", fmt.Sprintf("Failed to compute spec hash: %v", err))
		r.updateCondition(ctx, &policy, "Ready", metav1.ConditionFalse, "HashComputationFailed", err.Error())
		return ctrl.Result{}, err
	}

	// Update status if hashes have changed
	needsStatusUpdate := false
	if policy.Status.SelectorHash != selectorHash {
		log.Info("Selector hash changed", "old", policy.Status.SelectorHash, "new", selectorHash)
		policy.Status.SelectorHash = selectorHash
		needsStatusUpdate = true
		r.Recorder.Eventf(&policy, corev1.EventTypeNormal, "SelectorHashUpdated", "Selector hash updated to %s", selectorHash)
	}
	if policy.Status.SpecHash != specHash {
		log.Info("Spec hash changed", "old", policy.Status.SpecHash, "new", specHash)
		policy.Status.SpecHash = specHash
		needsStatusUpdate = true
		r.Recorder.Eventf(&policy, corev1.EventTypeNormal, "SpecHashUpdated", "Spec hash updated to %s", specHash)
	}
	if policy.Status.ObservedGeneration != policy.Generation {
		policy.Status.ObservedGeneration = policy.Generation
		needsStatusUpdate = true
	}

	// Update index with the policy
	r.Index.Upsert(&policy, selectorHash)
	log.Info("DnsPolicy indexed", "name", req.NamespacedName, "selectorHash", selectorHash, "specHash", specHash)
	r.Recorder.Event(&policy, corev1.EventTypeNormal, "PolicyIndexed", "DnsPolicy successfully indexed and ready")

	// Update status if needed
	if needsStatusUpdate {
		r.updateCondition(ctx, &policy, "Ready", metav1.ConditionTrue, "Reconciled", "DnsPolicy successfully reconciled")
		if err := r.Status().Update(ctx, &policy); err != nil {
			log.Error(err, "Failed to update status")
			r.Recorder.Event(&policy, corev1.EventTypeWarning, "StatusUpdateFailed", fmt.Sprintf("Failed to update status: %v", err))
			return ctrl.Result{}, err
		}
		r.Recorder.Event(&policy, corev1.EventTypeNormal, "Reconciled", "DnsPolicy successfully reconciled")
	}

	return ctrl.Result{}, nil
}

// updateCondition updates a condition in the policy status
func (r *DnsPolicyReconciler) updateCondition(ctx context.Context, policy *dnsv1alpha1.DnsPolicy,
	conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: policy.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}

	// Find and update existing condition, or append new one
	found := false
	for i, existing := range policy.Status.Conditions {
		if existing.Type == conditionType {
			// Only update if status changed
			if existing.Status != status {
				policy.Status.Conditions[i] = condition
			}
			found = true
			break
		}
	}
	if !found {
		policy.Status.Conditions = append(policy.Status.Conditions, condition)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *DnsPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Initialize the event recorder
	r.Recorder = mgr.GetEventRecorderFor("dnspolicy-controller")

	return ctrl.NewControllerManagedBy(mgr).
		For(&dnsv1alpha1.DnsPolicy{}).
		Named("dnspolicy").
		Complete(r)
}

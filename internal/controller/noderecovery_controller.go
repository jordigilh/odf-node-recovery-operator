/*
Copyright 2024.

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
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	odfv1alpha1 "github.com/jordigilh/odf-node-recovery-operator/api/v1alpha1"
	"github.com/jordigilh/odf-node-recovery-operator/internal/controller/pod"
	"github.com/jordigilh/odf-node-recovery-operator/monitoring"
	"k8s.io/client-go/tools/record"
)

// NodeRecoveryReconciler reconciles a NodeRecovery object
type NodeRecoveryReconciler struct {
	client.Client
	*rest.Config
	Scheme    *runtime.Scheme
	Recorder  record.EventRecorder
	CmdRunner pod.RemoteCommandExecutor
	LogClient podLogRetriever
}

// Node Recovery
//+kubebuilder:rbac:groups=odf.openshift.io,resources=noderecoveries,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=odf.openshift.io,resources=noderecoveries/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=odf.openshift.io,resources=noderecoveries/finalizers,verbs=update

// OCP
//+kubebuilder:rbac:groups=config.openshift.io,resources=clusterversions,verbs=get;list;watch
//+kubebuilder:rbac:groups=template.openshift.io,resources=templates,verbs=get;list;watch
//+kubebuilder:rbac:groups=ocs.openshift.io,resources=storageclusters;ocsinitializations,verbs=get;update;watch;list

//K8s
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;delete;watch;deletecollection
//+kubebuilder:rbac:groups="",resources=pods/exec,verbs=create
//+kubebuilder:rbac:groups="",resources=pods/log,verbs=get
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;update;watch
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;delete;list;create;watch
//+kubebuilder:rbac:groups="",resources=nodes,verbs=get;update;watch;list
//+kubebuilder:rbac:groups="",resources=persistentvolumes,verbs=get;list;watch;delete
//+kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch

// Events
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// +operator-sdk:csv:customresourcedefinitions:displayName="ODF Node Recovery"

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NodeRecovery object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile

func (r *NodeRecoveryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithName("odf-node-recovery-controller")

	log.Info("Reconciling NodeRecovery", "name", req.Name)
	// Lookup the instance for this reconcile request
	instance := &odfv1alpha1.NodeRecovery{}
	var err error

	if err = r.Get(ctx, req.NamespacedName, instance); client.IgnoreNotFound(err) != nil {
		log.Error(err, "Unable to fetch NodeRecovery", "name", req.Name)
		return ctrl.Result{}, err
	}

	log.Info("NodeRecovery fetched...", "name", instance.Name)
	if apierrors.IsNotFound(err) || !instance.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}
	if instance.Status.Phase == odfv1alpha1.FailedPhase ||
		instance.Status.Phase == odfv1alpha1.CompletedPhase {
		log.Info("Attempting to process CR", "name", instance.Name, "which is in", string(instance.Status.Phase), "Ignoring...")
		return ctrl.Result{}, nil
	}
	if instance.Status.StartTime.IsZero() {
		instance.Status.StartTime = &metav1.Time{Time: time.Now()}
		instance.Status.Phase = odfv1alpha1.RunningPhase
		instance.Status.Conditions = append(instance.Status.Conditions, odfv1alpha1.RecoveryCondition{Type: odfv1alpha1.EnableCephToolsPod, LastTransitionTime: metav1.Now(), Status: odfv1alpha1.StatusTrue})
		monitoring.TotalOperandInstancesPrometheusCounter.Inc()
	}
	recoverer, err := newNodeRecoveryReconciler(ctx, log, r.Client, r.Config, r.Scheme, r.Recorder, r.CmdRunner, r.LogClient)
	if err != nil {
		return ctrl.Result{}, err
	}
	result, err := recoverer.Reconcile(instance)
	serr := r.Status().Update(ctx, instance)
	return result, errors.Join(err, serr)
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodeRecoveryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &v1.Pod{}, podStatusPhaseFieldSelector, func(rawObj client.Object) []string {
		obj, ok := rawObj.(*v1.Pod)
		if !ok {
			return nil
		}
		return []string{string(obj.Status.Phase)}
	}); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &v1.PersistentVolumeClaim{}, pvcStatusPhaseFieldSelector, func(rawObj client.Object) []string {
		obj, ok := rawObj.(*v1.PersistentVolumeClaim)
		if !ok {
			return nil
		}
		return []string{string(obj.Status.Phase)}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&odfv1alpha1.NodeRecovery{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Owns(&odfv1alpha1.NodeRecovery{}).
		Complete(r)
}

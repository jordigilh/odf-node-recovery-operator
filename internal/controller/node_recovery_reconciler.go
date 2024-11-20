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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	version "github.com/hashicorp/go-version"
	"github.com/jordigilh/odf-node-recovery-operator/internal/controller/pod"
	odfv1alpha1 "github.com/jordigilh/odf-node-recovery-operator/pkg/api/v1alpha1"
	configv1 "github.com/openshift/api/config/v1"
	octemplateapi "github.com/openshift/api/template"
	templatev1 "github.com/openshift/api/template/v1"
	"github.com/openshift/library-go/pkg/template/generator"
	"github.com/openshift/library-go/pkg/template/templateprocessing"
	ocsoperatorv1 "github.com/red-hat-storage/ocs-operator/api/v1"
	"github.com/red-hat-storage/ocs-operator/controllers/defaults"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"k8s.io/client-go/kubernetes"

	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubernetes/pkg/kubelet"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/names"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// NodeRecovery represents the structure of the object that recovers an ODF cluster from a physical failure
type NodeRecovery struct {
	client.Client
	*rest.Config
	Scheme       *runtime.Scheme
	ctx          context.Context
	cmdRunner    pod.RemoteCommandExecutor
	recorder     record.EventRecorder
	log          logr.Logger
	logRetriever podLogRetriever
}

func newNodeRecoveryReconciler(
	ctx context.Context,
	log logr.Logger,
	client client.Client,
	restConfig *rest.Config,
	scheme *runtime.Scheme,
	recorder record.EventRecorder,
	cmdRunner pod.RemoteCommandExecutor,
	logRetriever podLogRetriever) (*NodeRecovery, error) {

	return &NodeRecovery{
		Client:       client,
		Config:       restConfig,
		ctx:          ctx,
		Scheme:       scheme,
		recorder:     recorder,
		cmdRunner:    cmdRunner,
		log:          log,
		logRetriever: logRetriever,
	}, nil
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *NodeRecovery) Reconcile(instance *odfv1alpha1.NodeRecovery) (ctrl.Result, error) {
	latestCondition := getLatestCondition(instance)
	if time.Now().After(latestCondition.LastTransitionTime.Time.Add(reconciliationTimeout)) {
		r.log.Error(fmt.Errorf("failed to reconcile after retrying for %.f minutes", reconciliationTimeout.Minutes()), "reason", latestCondition.Message)
		instance.Status.Phase = odfv1alpha1.FailedPhase
		r.recorder.Eventf(instance, "Error", "Reconciliation", fmt.Sprintf("failed to reconcile after retrying for 5 minutes: %s", latestCondition.Message))
		return ctrl.Result{}, nil
	}
	switch latestCondition.Type {
	case odfv1alpha1.EnableCephToolsPod:
		enabled, err := r.isCephToolsEnabled()
		if err != nil {
			r.log.Error(err, "failed to check if Ceph tools pod is enabled")
			latestCondition.Reason = odfv1alpha1.FailedCheckCephToolsPod
			latestCondition.Message = err.Error()
			return ctrl.Result{}, err
		}
		if !enabled {
			err = r.enableCephTools()
			if err != nil {
				r.log.Error(err, "failed to enable Ceph tools pod")
				latestCondition.Reason = odfv1alpha1.FailedEnableCephToolsPod
				latestCondition.Message = err.Error()
				return ctrl.Result{}, err
			}
		}
		transitionNextCondition(instance, odfv1alpha1.WaitForCephToolsPodRunning)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	case odfv1alpha1.WaitForCephToolsPodRunning:
		phase, err := r.getCephToolsPodPhase()
		if err != nil {
			r.log.Error(err, "failed to retrieve Ceph tools pod phase")
			latestCondition.Reason = odfv1alpha1.FailedRetrieveCephToolPod
			latestCondition.Message = err.Error()
			return ctrl.Result{}, err
		}
		if phase != v1.PodRunning {
			latestCondition.Reason = odfv1alpha1.PodNotInRunningPhase
			latestCondition.Message = "Ceph Tool pod is not in Running phase"
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		transitionNextCondition(instance, odfv1alpha1.WaitForOSDPodsStabilize)
		return ctrl.Result{Requeue: true}, nil
	case odfv1alpha1.WaitForOSDPodsStabilize:
		//  WAIT FOR PODS TO STABILIZE
		podErr, err := r.hasPodsInCreatingOrInitializingState()
		if err != nil {
			r.log.Error(err, "failed to retrieve list of pods in creating or initializing state")
			latestCondition.Reason = odfv1alpha1.FailedRetrievePodsPhase
			latestCondition.Message = err.Error()
			return ctrl.Result{}, err
		}
		if podErr != nil {
			// there are stil pods initializing... requeuing.
			latestCondition.Reason = odfv1alpha1.WaitingForPodsToInitialize
			latestCondition.Message = fmt.Sprintf("OSD pods still in initializing status: %v", podErr)
			r.log.Error(podErr, "Requeuing due to pods not yet initialized with")
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		transitionNextCondition(instance, odfv1alpha1.LabelNodesWithPendingPods)
		return ctrl.Result{Requeue: true}, nil
	case odfv1alpha1.LabelNodesWithPendingPods:
		pendingOSDPods, err := r.getPodsInPendingPhase()
		if err != nil {
			r.log.Error(err, "failed to retrieve list of pods in pending state")
			latestCondition.Reason = odfv1alpha1.FailedRetrievePendingPods
			latestCondition.Message = err.Error()
			return ctrl.Result{}, err
		}
		if len(pendingOSDPods.Items) > 0 {
			// PENDING STATE PODS
			instance.Status.PendingPods = true
			err = r.labelNodesWithPodsInPendingState(pendingOSDPods)
			if err != nil {
				r.log.Error(err, "failed to label nodes with pods in pending state")
				latestCondition.Reason = odfv1alpha1.FailedLabelNodes
				latestCondition.Message = err.Error()
				return ctrl.Result{}, err
			}
		}
		transitionNextCondition(instance, odfv1alpha1.ManageCrashLoopBackOffPods)
		return ctrl.Result{Requeue: true}, nil
	case odfv1alpha1.ManageCrashLoopBackOffPods:
		// INITCRASHLOOKBACKOFF STATE PODS
		osdPods, err := r.getOSDPodsInCrashLoopBackOff()
		if err != nil {
			r.log.Error(err, "failed to retrieve list of OSD specific pods in CrashLoopbackOff")
			latestCondition.Reason = odfv1alpha1.FailedRetrieveCrashLoopBackOffPods
			latestCondition.Message = err.Error()
			return ctrl.Result{}, err
		}
		if len(osdPods) == 0 {
			transitionNextCondition(instance, odfv1alpha1.RestartStorageOperator)
			return ctrl.Result{Requeue: true}, nil
		}
		instance.Status.CrashLoopBackOffPods = true
		nodeDevice, osdIDs, c, err := r.handleCrashLoopBackOffPods(osdPods)
		if err != nil {
			r.log.Error(err, "failed to handle pods in CrashLoopbackOff")
			latestCondition.Reason = odfv1alpha1.FailedHandleCrashLoopBackOffPods
			latestCondition.Message = err.Error()
			return c, err
		}
		instance.Status.NodeDevice = nodeDevice
		instance.Status.CrashedOSDDeploymentIDs = osdIDs
		transitionNextCondition(instance, odfv1alpha1.ForceDeleteRookCephOSDPods)
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	case odfv1alpha1.ForceDeleteRookCephOSDPods:
		if !instance.Status.CrashLoopBackOffPods {
			transitionNextCondition(instance, odfv1alpha1.RestartStorageOperator)
			return ctrl.Result{Requeue: true}, nil
		}
		for _, id := range instance.Status.CrashedOSDDeploymentIDs {
			pods, err := r.getOSDPodsWithID(id)
			if err != nil {
				return ctrl.Result{}, err
			}
			if len(pods.Items) > 0 {
				if time.Now().Before(pods.Items[0].DeletionTimestamp.Add(time.Minute)) {
					return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
				}
			}
		}
		// pods have been deleting for over a minute, force delete them.
		err := r.forceDeleteRookCephOSDPods(instance.Status.CrashedOSDDeploymentIDs)
		if err != nil {
			return ctrl.Result{}, err
		}
		err = r.deleteOldOSDRemovalJob()
		if err != nil {
			return ctrl.Result{}, err
		}
		osdPods, err := r.getOSDPods()
		if err != nil {
			return ctrl.Result{}, err
		}
		instance.Status.ForcedOSDRemoval = len(osdPods.Items) <= 3
		err = r.processOCSOSDRemovalTemplate(instance.Status.CrashedOSDDeploymentIDs, instance.Status.ForcedOSDRemoval)
		if err != nil {
			return ctrl.Result{}, err
		}
		transitionNextCondition(instance, odfv1alpha1.CleanupOSDRemovalJob)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil

	case odfv1alpha1.CleanupOSDRemovalJob:
		pod, err := r.getOSDRemovalPodJobCompletionStatus()
		if err != nil {
			r.log.Error(err, "failed to retrieve the OSD removal pod job")
			latestCondition.Message = err.Error()
			return ctrl.Result{}, err
		}
		if pod.Status.Phase != v1.PodSucceeded {
			if time.Now().Before(latestCondition.LastTransitionTime.Time.Add(osdRemovalJobTimeout)) {
				return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
			}
			r.log.Error(fmt.Errorf("failed to reconcile after retrying for %.f minutes", osdRemovalJobTimeout.Minutes()), "timed out waiting for the OSD removal job to complete")
			r.recorder.Eventf(instance, "Warning", "Reconciliation", fmt.Sprintf("OSD removal job timed out after %.f minutes. Retrying with %s=%t", osdRemovalJobTimeout.Minutes(), FORCE_OSD_REMOVAL, enableForcedOSDRemoval))
			err = r.deleteOldOSDRemovalJob()
			if err != nil {
				r.log.Error(err, "failed to delete OSD removal jobs")
				return ctrl.Result{}, err
			}
			err = r.processOCSOSDRemovalTemplate(instance.Status.CrashedOSDDeploymentIDs, enableForcedOSDRemoval)
			if err != nil {
				return ctrl.Result{}, err
			}
			instance.Status.ForcedOSDRemoval = true
			transitionNextCondition(instance, odfv1alpha1.RetryForceCleanupOSDRemovalJob)
			return ctrl.Result{Requeue: true}, nil
		}
		if err = r.validateJobLogs(pod.Name); err != nil {
			latestCondition.Message = err.Error()
			instance.Status.Phase = odfv1alpha1.FailedPhase
			return ctrl.Result{}, err
		}
		err = r.deleteOldOSDRemovalJob()
		if err != nil {
			r.log.Error(err, "failed to delete preexisting OSD removal jobs")
			return ctrl.Result{}, err
		}
		transitionNextCondition(instance, odfv1alpha1.DeletePersistentVolume)
		return ctrl.Result{Requeue: true}, nil
	case odfv1alpha1.RetryForceCleanupOSDRemovalJob:
		pod, err := r.getOSDRemovalPodJobCompletionStatus()
		if err != nil {
			r.log.Error(err, "failed to retrieve the OSD removal pod job")
			latestCondition.Message = err.Error()
			return ctrl.Result{}, err
		}
		if pod.Status.Phase != v1.PodSucceeded {
			if time.Now().Before(latestCondition.LastTransitionTime.Time.Add(osdRemovalJobTimeout)) {
				return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
			}
			r.log.Error(fmt.Errorf("failed to reconcile after retrying for %.f minutes with enabled forced OSD Removal flag", osdRemovalJobTimeout.Minutes()), "timed out waiting for the OSD removal job to complete")
			r.recorder.Eventf(instance, "Warning", "Reconciliation", fmt.Sprintf("OSD removal job timed out after %.f minutes.", osdRemovalJobTimeout.Minutes()))
			instance.Status.Phase = odfv1alpha1.FailedPhase
			return ctrl.Result{}, nil
		}
		// retrieve the pod logs and validate that it completed successfully
		if err = r.validateJobLogs(pod.Name); err != nil {
			latestCondition.Message = err.Error()
			instance.Status.Phase = odfv1alpha1.FailedPhase
			return ctrl.Result{}, err
		}
		err = r.deleteOldOSDRemovalJob()
		if err != nil {
			r.log.Error(err, "failed to delete preexisting OSD removal jobs")
			return ctrl.Result{}, err
		}
		transitionNextCondition(instance, odfv1alpha1.DeletePersistentVolume)
		return ctrl.Result{Requeue: true}, nil
	case odfv1alpha1.DeletePersistentVolume:
		for _, nd := range instance.Status.NodeDevice {
			err := r.deletePV(nd.PersistentVolumeName)
			if err != nil {
				r.log.Error(fmt.Errorf("failed to delete PV"), "pv", nd.PersistentVolumeName)
				latestCondition.Message = fmt.Sprintf("failed to delete PV %s in node %s", nd.PersistentVolumeName, nd.NodeName)
			}
		}
		transitionNextCondition(instance, odfv1alpha1.RestartStorageOperator)
		return ctrl.Result{Requeue: true}, nil
	case odfv1alpha1.RestartStorageOperator:
		if instance.Status.PendingPods || instance.Status.CrashLoopBackOffPods {
			err := r.deleteRookCephOperatorPod()
			if err != nil {
				r.log.Error(err, "failed to restart the storage operator")
				latestCondition.Reason = odfv1alpha1.FailedRestartODFOperator
				latestCondition.Message = err.Error()
				return ctrl.Result{}, err
			}
		}
		transitionNextCondition(instance, odfv1alpha1.DeleteFailedPodsNodeAffinity)
		return ctrl.Result{Requeue: true}, nil
	case odfv1alpha1.DeleteFailedPodsNodeAffinity:
		err := r.deleteFailedPodsWithReasonNodeAffinity()
		if err != nil {
			r.log.Error(err, "failed to delete failed pods with reason node affinity")
			latestCondition.Reason = odfv1alpha1.FailedDeleteFailedPodsNodeAffinity
			latestCondition.Message = err.Error()
			return ctrl.Result{}, err
		}
		err = r.archiveCephDaemonCrashMessages()
		if err != nil {
			r.log.Error(err, "failed to archive ceph daemon crash messages")
			latestCondition.Reason = odfv1alpha1.FailedArchiveCephDaemonCrashMessages
			latestCondition.Message = err.Error()
			return ctrl.Result{}, err
		}
		transitionNextCondition(instance, odfv1alpha1.StorageClusterFitnessCheck)
		return ctrl.Result{Requeue: true}, nil
	case odfv1alpha1.StorageClusterFitnessCheck:
		status, err := r.getCephHealthStatus()
		if err != nil {
			r.log.Error(err, "failed to get Ceph health status")
			latestCondition.Reason = odfv1alpha1.FailedRetrieveCephHealthStatus
			latestCondition.Message = err.Error()
			return ctrl.Result{}, err
		}
		if status != HEALTH_OK {
			latestCondition.Message = fmt.Sprintf("Waiting for cluster to become healthy: %s", status)
			return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
		}
		transitionNextCondition(instance, odfv1alpha1.DisableCephTools)
		return ctrl.Result{Requeue: true}, nil
	case odfv1alpha1.DisableCephTools:
		err := r.disableCephTools()
		if err != nil {
			r.log.Error(err, "failed to disable Ceph tools")
			latestCondition.Reason = odfv1alpha1.FailedDisableCephToolsPod
			latestCondition.Message = err.Error()
			return ctrl.Result{}, err
		}
	}
	instance.Status.Phase = odfv1alpha1.CompletedPhase
	instance.Status.CompletionTime = &metav1.Time{Time: time.Now()}
	r.recorder.Eventf(instance, "Normal", "Reconciliation", "Successfully completed recovering cluster in %s", instance.Status.CompletionTime.Sub(instance.Status.StartTime.Time))
	return ctrl.Result{}, nil
}

// handleCrashLoopBackOffPods identifies the OSD pods that are in CrashLoopbackOff status. The function then proceeds to
// scale the deployment of these pods to 0 replicas. The function returns a structure that contains the node name,
// PV and the OSD id associated to these pods
func (r *NodeRecovery) handleCrashLoopBackOffPods(osdPods []v1.Pod) ([]odfv1alpha1.NodePV, []string, reconcile.Result, error) {
	nodePV := []odfv1alpha1.NodePV{}
	osdIDs := []string{}
	for _, p := range osdPods {
		nodeDevice, err := r.getNodeDeviceNameFromPV(&p)
		if err != nil {
			return nil, nil, ctrl.Result{}, err
		}
		nodePV = append(nodePV, *nodeDevice)
		osdID, ok := p.Labels[osd.OsdIdLabelKey]
		if !ok {
			return nil, nil, ctrl.Result{}, fmt.Errorf("cannot determine OSD deployment object: missing label %s in pod %s/%s", p.Labels[osd.OsdIdLabelKey], p.Namespace, p.Name)
		}
		osdIDs = append(osdIDs, osdID)
		err = r.scaleRookCephOSDDeploymentToZero(osdID)
		if err != nil {
			return nil, nil, ctrl.Result{}, err
		}
	}
	return nodePV, osdIDs, ctrl.Result{RequeueAfter: 15 * time.Second}, nil
}

// hasPodsInCreatingOrInitializingState checks for pods that are in the creating state or initializing and returns an error that contains all references to the pods
func (r *NodeRecovery) hasPodsInCreatingOrInitializingState() (error, error) {
	pods := &v1.PodList{}
	err := r.List(r.ctx, pods, &client.ListOptions{Namespace: ODF_NAMESPACE})
	if err != nil {
		return nil, err
	}
	var errs error
	for _, pod := range pods.Items {
		for _, status := range append(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses...) {
			if status.State.Waiting != nil &&
				((status.State.Waiting.Reason == kubelet.ContainerCreating) || (status.State.Waiting.Reason == kubelet.PodInitializing)) {
				errs = errors.Join(errs, fmt.Errorf("pod %s: container %s waiting in %s: %s", pod.Name, status.Name, status.State.Waiting.Reason, status.State.Waiting.Message))
			}
		}
	}
	return errs, nil
}

// getStorageCluster returns the StorageCluster object instance named "ocs-storagecluster". This object
// is used in OCP >4.15 to manage the ceph tools pod
func (r *NodeRecovery) getStorageCluster() (*ocsoperatorv1.StorageCluster, error) {
	sc := &ocsoperatorv1.StorageCluster{}
	err := r.Get(r.ctx, types.NamespacedName{Namespace: ODF_NAMESPACE, Name: "ocs-storagecluster"}, sc, &client.GetOptions{})
	return sc, err
}

// getOCPVersion returns the OCP semver of the running cluster
func (r *NodeRecovery) getOCPVersion() (*version.Version, error) {
	c := &configv1.ClusterVersion{}
	err := r.Get(r.ctx, types.NamespacedName{Name: "version"}, c)
	if err != nil {
		return nil, err
	}
	for _, i := range c.Status.History {
		if i.State == configv1.CompletedUpdate {
			return version.NewVersion(i.Version)
		}
	}
	return nil, fmt.Errorf("no valid version found in clusterversion object")
}

// getOSCInitialization returns the OCSInitialization object instance named "ocsinit". This object
// is used in OCP <=4.14 to manage the ceph tools pod
func (r NodeRecovery) getOSCInitialization() (*ocsoperatorv1.OCSInitialization, error) {
	o := &ocsoperatorv1.OCSInitialization{}
	err := r.Get(r.ctx, types.NamespacedName{Namespace: ODF_NAMESPACE, Name: "ocsinit"}, o, &client.GetOptions{})
	return o, err
}

// getPodsInPendingPhase returns a podList object that contains all the ODF pods in the openshift-storage namespace
// that are in Pending phase
func (r *NodeRecovery) getPodsInPendingPhase() (*v1.PodList, error) {
	l := &v1.PodList{}
	err := r.List(r.ctx, l, &client.ListOptions{Namespace: ODF_NAMESPACE}, &client.MatchingFields{podStatusPhaseFieldSelector: string(v1.PodPending)}, &client.MatchingLabels{"app": "rook-ceph-osd"})
	if err != nil {
		return nil, err
	}
	return l, nil
}

// getPodsInFailedPhaseWithReasonNodeAffinity returns a slice of pods that are in failed phase and the reason is due to NodeAffinity
func (r *NodeRecovery) getPodsInFailedPhaseWithReasonNodeAffinity() ([]v1.Pod, error) {
	l := &v1.PodList{}
	failedPods := []v1.Pod{}
	err := r.List(r.ctx, l, &client.ListOptions{Namespace: ODF_NAMESPACE}, &client.MatchingFields{podStatusPhaseFieldSelector: string(v1.PodFailed)})
	if err != nil {
		return nil, err
	}
	for _, pod := range l.Items {
		if pod.Status.Reason == names.NodeAffinity {
			failedPods = append(failedPods, pod)
		}
	}
	return failedPods, nil
}

// deleteFailedPodsWithReasonNodeAffinity deletes pods that are in failed phase and the reason is due to NodeAffinity
func (r *NodeRecovery) deleteFailedPodsWithReasonNodeAffinity() error {
	failedPods, err := r.getPodsInFailedPhaseWithReasonNodeAffinity()
	if err != nil {
		return err
	}
	var errs error
	for _, pod := range failedPods {
		err = r.Delete(r.ctx, &pod, &client.DeleteOptions{})
		if err != nil {
			errs = errors.Join(errs, err)
		}
	}
	return errs
}

// labelNodesWithPodsInPendingState labels the nodes where pods are in pending state
func (r *NodeRecovery) labelNodesWithPodsInPendingState(pods *v1.PodList) error {
	nodes := map[string]struct{}{}
	for _, p := range pods.Items {
		nodes[p.Spec.NodeSelector[v1.LabelHostname]] = struct{}{}
	}
	var errs error
	for nodeName := range nodes {
		n := &v1.Node{}
		err := r.Get(r.ctx, types.NamespacedName{Name: nodeName}, n, &client.GetOptions{})
		if err != nil {
			errs = errors.Join(errs, err)
			continue
		}
		if n.Labels == nil {
			n.Labels = make(map[string]string)
		}
		n.Labels[defaults.NodeAffinityKey] = ""
		err = r.Update(r.ctx, n, &client.UpdateOptions{})
		if err != nil {
			errs = errors.Join(errs, err)
		}
	}
	return errs
}

// getOSDPodsInCrashLoopBackOff returns a list of OSD pods whose reason is CrashLoopbackOff. These
// pods have a label that matches the `app=rook-ceph-osd` condition.
func (r *NodeRecovery) getOSDPodsInCrashLoopBackOff() ([]v1.Pod, error) {
	pods := []v1.Pod{}
	l, err := r.getOSDPods()
	if err != nil {
		return nil, err
	}
	for _, pod := range l.Items {
		for _, status := range append(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses...) {
			if status.State.Waiting != nil && (status.State.Waiting.Reason == podStateReasonCrashLoopBackOff) {
				pods = append(pods, pod)
			}
		}
	}
	return pods, nil
}

// getNodeDeviceNameFromPV returns a struct that contain the node and the PV used by the OSD pod
func (r *NodeRecovery) getNodeDeviceNameFromPV(pod *v1.Pod) (*odfv1alpha1.NodePV, error) {
	if pvcName, ok := pod.Labels[osd.OSDOverPVCLabelKey]; ok {
		pvc := &v1.PersistentVolumeClaim{}
		err := r.Get(r.ctx, types.NamespacedName{Namespace: pod.Namespace, Name: pvcName}, pvc, &client.GetOptions{})
		if err != nil {
			return nil, err
		}
		pv := &v1.PersistentVolume{}
		err = r.Get(r.ctx, types.NamespacedName{Name: pvc.Spec.VolumeName}, pv, &client.GetOptions{})
		if err != nil {
			return nil, err
		}
		return &odfv1alpha1.NodePV{NodeName: pv.Labels[v1.LabelHostname], PersistentVolumeName: pv.Name}, nil
	}
	return nil, nil
}

// processOCSOSDRemovalTemplate triggers the processing of the OSD removal job template and its job. It accepts
// a list of the OSD IDs to be removed and whether to force the removal or not. These parameters are then passed
// to the template for processing.
func (r *NodeRecovery) processOCSOSDRemovalTemplate(ids []string, forceOSDRemovalParam bool) error {
	t := &templatev1.Template{}
	err := r.Get(r.ctx, types.NamespacedName{Namespace: ODF_NAMESPACE, Name: OCS_OSD_REMOVAL_JOB}, t)
	if err != nil {
		return err
	}
	failedOSDIDs := templateprocessing.GetParameterByName(t, FAILED_OSD_IDS)
	failedOSDIDs.Value = strings.Join(ids, ",")
	forcedRemoval := templateprocessing.GetParameterByName(t, FORCE_OSD_REMOVAL)
	forcedRemoval.Value = strconv.FormatBool(forceOSDRemovalParam)
	processor := templateprocessing.NewProcessor(map[string]generator.Generator{
		"expression": generator.NewExpressionValueGenerator(rand.New(rand.NewSource(time.Now().UnixNano()))),
	})
	if errs := processor.Process(t); len(errs) > 0 {
		return kerrors.NewInvalid(octemplateapi.Kind("Template"), t.Name, errs)
	}
	// attempt to convert our resulting object to external
	for _, obj := range t.Objects {
		objToCreate := obj.Object
		if objToCreate == nil {
			converted, err := runtime.Decode(unstructured.UnstructuredJSONScheme, obj.Raw)
			if err != nil {
				return err
			}
			objToCreate = converted
		}
		var (
			cobj client.Object
			ok   bool
		)
		if cobj, ok = objToCreate.(client.Object); !ok {
			return fmt.Errorf("failed to cast as client.Object: %v", objToCreate)
		}
		// Add namespace because it's stripped from the template at processing time
		// https://github.com/openshift/library-go/blob/144cb72bbb3903cd74ce307dc0688ce37b45b97e/pkg/template/templateprocessing/template.go#L86-L90
		cobj.SetNamespace(ODF_NAMESPACE)
		if err := r.Create(r.ctx, cobj, &client.CreateOptions{}); err != nil {
			return err
		}
	}
	return nil
}

// getOSDRemovalPodCompletionStatus retrieves the OSD removal job pod
func (r *NodeRecovery) getOSDRemovalPodJobCompletionStatus() (*v1.Pod, error) {
	pods := &v1.PodList{}
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{Key: "job-name", Operator: metav1.LabelSelectorOpIn, Values: []string{"ocs-osd-removal-job"}},
		}},
	)
	if err != nil {
		return nil, err
	}
	err = r.List(r.ctx, pods, &client.ListOptions{LabelSelector: selector, Namespace: ODF_NAMESPACE})
	if err != nil {
		return nil, err
	}
	for _, p := range pods.Items {
		if p.ObjectMeta.DeletionTimestamp.IsZero() {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("failed to retrieve the status of the ODF removal job pod in %s: no pod found or all pod instances are being deleted", ODF_NAMESPACE)
}

// deleteRookCephOperatorPod deletes the rook ceph operator pod instance
func (r *NodeRecovery) deleteRookCephOperatorPod() error {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{Key: "app", Operator: metav1.LabelSelectorOpIn, Values: []string{"rook-ceph-operator"}},
		}},
	)
	if err != nil {
		return err
	}
	return r.DeleteAllOf(r.ctx, &v1.Pod{}, &client.DeleteAllOfOptions{ListOptions: client.ListOptions{Namespace: ODF_NAMESPACE, LabelSelector: selector}})
}

type podLogRetriever interface {
	GetLogs(podName string) (string, error)
}

type LogClient struct {
	clientset *kubernetes.Clientset
}

func NewLogRetriever(config *rest.Config) podLogRetriever {
	return &LogClient{clientset: kubernetes.NewForConfigOrDie(config)}
}

// getPodLogs retrieves the logs of a pod in the openshift-storage namespace
func (l *LogClient) GetLogs(podName string) (string, error) {
	podLogOpts := v1.PodLogOptions{}
	req := l.clientset.CoreV1().Pods(ODF_NAMESPACE).GetLogs(podName, &podLogOpts)
	podLogs, err := req.Stream(context.TODO())
	if err != nil {
		return "", fmt.Errorf("error in opening stream: %s", err)
	}
	defer podLogs.Close()
	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		return "", fmt.Errorf("error in copy information from podLogs to buffer: %s", err)
	}
	return buf.String(), nil
}

// deletePV deletes a PV from the cluster specified by the pvName parameter
func (r *NodeRecovery) deletePV(pvName string) error {
	pv := &v1.PersistentVolume{}
	err := r.Get(r.ctx, types.NamespacedName{Name: pvName}, pv, &client.GetOptions{})
	if err != nil {
		if !kerrors.IsNotFound(err) {
			return err
		}
		return nil
	}
	return r.Delete(r.ctx, pv, &client.DeleteOptions{})
}

// validateJobLogs checks the logs of the OSD removal job to ensure that the logs show
// a successful run
func (r *NodeRecovery) validateJobLogs(podName string) error {
	logs, err := r.logRetriever.GetLogs(podName)
	if err != nil {
		return err
	}
	logs = strings.TrimSpace(logs) // remove empty last l
	slogs := strings.Split(logs, "\n")
	if s := strings.ToLower(slogs[len(slogs)-1]); !strings.Contains(s, osdJobSuccessMessage) {
		r.log.Error(fmt.Errorf("osd job removal completed with failure"), slogs[len(slogs)-1])
		return fmt.Errorf("osd job removal completed with failure: %s", slogs[len(slogs)-1])
	}
	return nil
}

// getLatestCondition returns the latest condition
func getLatestCondition(instance *odfv1alpha1.NodeRecovery) *odfv1alpha1.RecoveryCondition {
	latest := &instance.Status.Conditions[len(instance.Status.Conditions)-1]
	latest.LastProbeTime = metav1.Now()
	return latest
}

// transitionNextCondition adds the new condition to the slice and flags the current one as false so that the CLI command
// shows the newly added condition as the current
func transitionNextCondition(instance *odfv1alpha1.NodeRecovery, nextCondition odfv1alpha1.RecoveryConditionType) {
	instance.Status.Conditions[len(instance.Status.Conditions)-1].Status = odfv1alpha1.StatusFalse
	instance.Status.Conditions = append(instance.Status.Conditions, odfv1alpha1.RecoveryCondition{Type: nextCondition, LastTransitionTime: metav1.Now(), LastProbeTime: metav1.Now(), Status: odfv1alpha1.StatusTrue})
}

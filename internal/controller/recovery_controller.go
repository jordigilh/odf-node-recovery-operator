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
	"fmt"
	"math/rand"
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
	localv1 "github.com/openshift/local-storage-operator/pkg/common"
	ocsoperatorv1 "github.com/red-hat-storage/ocs-operator/api/v1"
	"github.com/red-hat-storage/ocs-operator/controllers/defaults"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"

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
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// NodeRecovery represents the structure of the object that recovers an ODF cluster from a physical failure
type NodeRecovery struct {
	client.Client
	*rest.Config
	Scheme    *runtime.Scheme
	ctx       context.Context
	cmdRunner pod.RemoteCommandExecutor
	recorder  record.EventRecorder
	log       logr.Logger
}

func newNodeRecoveryReconciler(ctx context.Context, client client.Client, restConfig *rest.Config, scheme *runtime.Scheme, recorder record.EventRecorder, cmdRunner pod.RemoteCommandExecutor) (*NodeRecovery, error) {

	return &NodeRecovery{
		Client:    client,
		Config:    restConfig,
		ctx:       ctx,
		Scheme:    scheme,
		recorder:  recorder,
		cmdRunner: cmdRunner,
		log:       log.FromContext(ctx).WithValues("odf-node-reconciler"),
	}, nil
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Pod object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *NodeRecovery) Reconcile(instance *odfv1alpha1.NodeRecovery) (ctrl.Result, error) {

	latestCondition := getLatestCondition(instance)
	if time.Now().After(latestCondition.LastTransitionTime.Time.Add(5 * time.Minute)) {
		r.log.Error(fmt.Errorf("failed to reconcile after retrying for 5 minutes: %s", latestCondition.Message), "failed to reconcile")
		instance.Status.Phase = odfv1alpha1.FailedPhase
		r.recorder.Eventf(instance, "Error", "Reconciliation", fmt.Sprintf("failed to reconcile after retrying for 5 minutes: %s", latestCondition.Message))
		return ctrl.Result{}, nil
	}
	switch latestCondition.Type {
	case odfv1alpha1.EnableCephToolsPod:
		enabled, err := r.isCephToolsEnabled()
		if err != nil {
			latestCondition.Reason = odfv1alpha1.FailedCheckCephToolsPod
			latestCondition.Message = err.Error()
			return ctrl.Result{}, err
		}
		if !enabled {
			err = r.enableCephTools()
			if err != nil {
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
			latestCondition.Reason = odfv1alpha1.FailedRetrievePodsPhase
			latestCondition.Message = err.Error()
			return ctrl.Result{}, err
		}
		if podErr != nil {
			// there are stil pods initializing... requeuing.
			latestCondition.Reason = odfv1alpha1.WaitingForPodsToInitialize
			latestCondition.Message = fmt.Sprintf("OSD pods still in initializing status: %v", podErr)
			r.log.V(5).Info("Requeuing due to pods not yet initialized: %v", podErr)
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		transitionNextCondition(instance, odfv1alpha1.LabelNodesWithPendingPods)
		return ctrl.Result{Requeue: true}, nil
	case odfv1alpha1.LabelNodesWithPendingPods:
		pendingOSDPods, err := r.getPodsInPendingPhase()
		if err != nil {
			latestCondition.Reason = odfv1alpha1.FailedRetrievePendingPods
			latestCondition.Message = err.Error()
			return ctrl.Result{}, err
		}
		if len(pendingOSDPods.Items) > 0 {
			// PENDING STATE PODS
			instance.Status.PendingPods = true
			err = r.labelNodesWithPodsInPendingState(pendingOSDPods)
			if err != nil {
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
			latestCondition.Reason = odfv1alpha1.FailedRetrieveCrashLoopBackOffPods
			latestCondition.Message = err.Error()
			return ctrl.Result{}, err
		}
		if len(osdPods) > 0 {
			instance.Status.CrashLoopBackOffPods = true
			c, err := r.handleCrashLoopBackOffPods(osdPods)
			if err != nil {
				latestCondition.Reason = odfv1alpha1.FailedHandleCrashLoopBackOffPods
				latestCondition.Message = err.Error()
				return c, err
			}
			transitionNextCondition(instance, odfv1alpha1.CleanupOSDRemovalJob)
			return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
		}
		transitionNextCondition(instance, odfv1alpha1.RestartStorageOperator)
		return ctrl.Result{Requeue: true}, nil
	case odfv1alpha1.CleanupOSDRemovalJob:
		pod, err := r.getOSDRemovalPodJobCompletionStatus()
		if err != nil {
			return ctrl.Result{}, err
		}
		if pod.Status.Phase != v1.PodSucceeded {
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		err = r.deleteOldOSDRemovalJob()
		if err != nil {
			return ctrl.Result{}, err
		}
		transitionNextCondition(instance, odfv1alpha1.RestartStorageOperator)
		return ctrl.Result{Requeue: true}, nil
	case odfv1alpha1.RestartStorageOperator:
		if instance.Status.PendingPods || instance.Status.CrashLoopBackOffPods {
			err := r.restartStorageOperator()
			if err != nil {
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
			latestCondition.Reason = odfv1alpha1.FailedDeleteFailedPodsNodeAffinity
			latestCondition.Message = err.Error()
			return ctrl.Result{}, err
		}
		err = r.archiveCephDaemonCrashMessages()
		if err != nil {
			latestCondition.Reason = odfv1alpha1.FailedArchiveCephDaemonCrashMessages
			latestCondition.Message = err.Error()
			return ctrl.Result{}, err
		}
		transitionNextCondition(instance, odfv1alpha1.StorageClusterFitnessCheck)
		return ctrl.Result{Requeue: true}, nil
	case odfv1alpha1.StorageClusterFitnessCheck:
		status, err := r.getCephHealthStatus()
		if err != nil {
			latestCondition.Reason = odfv1alpha1.FailedRetrieveCephHealthStatus
			latestCondition.Message = err.Error()
			return ctrl.Result{}, err
		}
		if status != HEALTH_OK {
			return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
		}
		transitionNextCondition(instance, odfv1alpha1.DisableCephTools)
		return ctrl.Result{Requeue: true}, nil
	case odfv1alpha1.DisableCephTools:
		err := r.disableCephTools()
		if err != nil {
			latestCondition.Reason = odfv1alpha1.FailedDisableCephToolsPod
			latestCondition.Message = err.Error()
			return ctrl.Result{}, err
		}
	}
	instance.Status.Phase = odfv1alpha1.CompletedPhase
	instance.Status.CompletionTime = &metav1.Time{Time: time.Now()}
	r.recorder.Eventf(instance, "Info", "Reconciliation", "Successfully completed recovering cluster in %s", instance.Status.CompletionTime.Sub(instance.Status.StartTime.Time))
	return ctrl.Result{}, nil
}

func (r *NodeRecovery) handleCrashLoopBackOffPods(osdPods []v1.Pod) (reconcile.Result, error) {
	diskDevices := []nodeDevice{}

	for _, p := range osdPods {
		nodeDevice, err := r.getNodeDeviceNameFromPV(&p)
		if err != nil {
			return ctrl.Result{}, err
		}
		diskDevices = append(diskDevices, *nodeDevice)
	}
	err := r.scaleRookCephOSDDeploymentsToZero()
	if err != nil {
		return ctrl.Result{}, err
	}
	pods, err := r.getRookCephOSDPods()
	if err != nil {
		return ctrl.Result{}, err
	}
	if len(pods.Items) > 0 {
		if time.Now().After(pods.Items[0].DeletionTimestamp.Add(time.Minute)) {
			return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
		}
		// pods have been deleting for over a minute, force delete them.
		err = r.forceDeleteRookCephOSDPods()
		if err != nil {
			return ctrl.Result{}, err
		}
	}
	//Erase devices before adding to OCS cluster
	for _, dd := range diskDevices {
		err = r.eraseDevice(&dd)
		if err != nil {
			return ctrl.Result{}, err
		}
	}
	err = r.deleteOldOSDRemovalJob()
	if err != nil {
		return ctrl.Result{}, err
	}
	err = r.processOCSOSDRemovalTemplate(osdPods)
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// hasPodsInCreatingOrInitializingState checks for pods that are in the creating state or initializing and returns an error that contains all references to the pods
// #
// # WAIT FOR PODS TO STABILIZE
// #
//   - name: Waiting for OCS pods' states to stabilize as Running, Pending, or error condition
//     shell: "oc get -n openshift-storage pods --no-headers"
//     register: osd_pods
//     # until: (osd_pods.stdout | regex_search('Init:[0-9]+\/[0-9]+') == none) and (osd_pods.stdout | regex_search('ContainerCreating') == none) and (osd_pods.stdout | regex_search('PodInitializing') == none)
//     until:
//   - osd_pods.stdout | regex_search('Init:[0-9]+\/[0-9]+') == none
//   - osd_pods.stdout | regex_search('ContainerCreating') == none
//   - osd_pods.stdout | regex_search('PodInitializing') == none
//     retries: 60
//     delay: 10
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

func (r *NodeRecovery) getStorageCluster() (*ocsoperatorv1.StorageCluster, error) {
	sc := &ocsoperatorv1.StorageCluster{}
	err := r.Get(r.ctx, types.NamespacedName{Namespace: ODF_NAMESPACE, Name: "ocs-storagecluster"}, sc, &client.GetOptions{})
	return sc, err
}

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
func (r NodeRecovery) getOSCInitialization() (*ocsoperatorv1.OCSInitialization, error) {
	o := &ocsoperatorv1.OCSInitialization{}
	err := r.Get(r.ctx, types.NamespacedName{Namespace: ODF_NAMESPACE, Name: "ocsinit"}, o, &client.GetOptions{})
	return o, err
}

func (r *NodeRecovery) getPodsInPendingPhase() (*v1.PodList, error) {
	l := &v1.PodList{}
	err := r.List(r.ctx, l, &client.ListOptions{Namespace: ODF_NAMESPACE}, &client.MatchingFields{podStatusPhaseFieldSelector: string(v1.PodPending)}, &client.MatchingLabels{"app": "rook-ceph-osd"})
	if err != nil {
		return nil, err
	}
	return l, nil
}

// STORAGE CLUSTER FITNESS CHECK
//
// getPodsInFailedPhaseWithReasonNodeAffinity retrieves a slice of pods that are in failed phase and the reason is due to NodeAffinity
//   - name: Get pods stuck in NodeAffinity status
//     shell: "oc get -n openshift-storage pods --field-selector=status.phase==\"Failed\" -o jsonpath='{range .items[?(@.status.reason==\"NodeAffinity\")]}{.metadata.name}{\"\\n\"}{end}'"
//     register: nodeaffinity_pods
//     until: nodeaffinity_pods.rc == 0
//     retries: 30
//     delay: 10
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
//   - name: Delete any pods stuck in NodeAffinity status
//     shell: "oc delete pod {{ item }} -n openshift-storage"
//     register: result
//     with_items: "{{ nodeaffinity_pods.stdout_lines }}"
//     when: 'nodeaffinity_pods.stdout_lines | length > 0'
//     until: result.rc == 0
//     retries: 30
//     delay: 10
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

// PENDING STATE PODS
//
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

// INITCRASHLOOKBACKOFF STATE PODS
//   - name: Get pods stuck in CrashLoopBackOff or Error status
//     shell: "oc get -n openshift-storage pods -l app=rook-ceph-osd --no-headers | grep 'CrashLoopBackOff\\|Error' | awk '{print $1}'"
//     register: crash_pods
//     until: crash_pods.rc == 0
//     retries: 30
//     delay: 10
func (r *NodeRecovery) getOSDPodsInCrashLoopBackOff() ([]v1.Pod, error) {
	l := &v1.PodList{}
	pods := []v1.Pod{}
	selectorMap := map[string]string{"app": "rook-ceph-osd"}
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchLabels: selectorMap})
	if err != nil {
		return nil, err
	}
	err = r.List(r.ctx, l, &client.ListOptions{Namespace: ODF_NAMESPACE, LabelSelector: selector})
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

type nodeDevice struct {
	nodeName   string
	deviceName string
}

// getNodeDeviceNameFromPV returns a struct that contain the node and device name associated to the PV used by the pod
// - name: Get pvc's
//   shell: "oc get -n openshift-storage pod/{{ item }} -o jsonpath='{.metadata.labels.ceph\\.rook\\.io\/pvc}'"
//   register: crash_pods_pvcs
//   until: crash_pods.rc == 0
//   with_items: "{{ crash_pods.stdout_lines }}"
//   retries: 30
//   delay: 10

// - name: Get pv's
//   shell: "oc get -n openshift-storage pvc/{{ item.stdout }} -o jsonpath='{.spec.volumeName}'"
//   register: crash_pods_pvs
//   until: crash_pods_pvs.rc == 0
//   with_items: "{{ crash_pods_pvcs.results }}"
//   retries: 30
//   delay: 10

// - name: Get devices
//   shell: "oc get pv/{{ item.stdout }} -o jsonpath='{.metadata.labels.kubernetes\\.io/hostname}{\",\"}{.metadata.annotations.storage\\.openshift\\.com\/device-name}'"
//   register: osd_pods_devices
//   until: osd_pods_devices.rc == 0
//   with_items: "{{ crash_pods_pvs.results }}"
//   retries: 30
//   delay: 10

func (r *NodeRecovery) getNodeDeviceNameFromPV(pod *v1.Pod) (*nodeDevice, error) {
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
		if dn, ok := pv.Annotations[localv1.PVDeviceNameLabel]; ok {
			return &nodeDevice{nodeName: pvc.Labels[v1.LabelHostname], deviceName: dn}, nil
		}
		return nil, fmt.Errorf("annotation %s not found in PV %s", localv1.PVDeviceNameLabel, pv.Name)
	}
	return nil, nil
}

//   - name: Perm delete osd's
//     shell: "oc process -n openshift-storage ocs-osd-removal -p FAILED_OSD_IDS={{ osds | join(',') }} |oc create -n openshift-storage -f -"
//     when: 'crash_pods.stdout_lines | length > 0'
//     retries: 30
//     delay: 10
//

// extractOSDIds returns a slice that contains the values of the label `osd.OsdIdLabelKey` in the pods.
func extractOSDIds(pods []v1.Pod) []string {
	ids := []string{}
	for _, p := range pods {
		ids = append(ids, p.Labels[osd.OsdIdLabelKey])
	}
	return ids
}

// proceessOCSOSDRemovalTemplate
//  - name: Perm delete osd's
//       shell: "oc process -n openshift-storage ocs-osd-removal -p FAILED_OSD_IDS={{ osds | join(',') }} |oc create -n openshift-storage -f -"
//       when: 'crash_pods.stdout_lines | length > 0'
//       retries: 30
//       delay: 10

func (r *NodeRecovery) processOCSOSDRemovalTemplate(crashingPods []v1.Pod) error {
	ids := extractOSDIds(crashingPods)
	t := &templatev1.Template{}
	err := r.Get(r.ctx, types.NamespacedName{Namespace: ODF_NAMESPACE, Name: OCS_OSD_REMOVAL}, t)
	if err != nil {
		return err
	}
	param := templateprocessing.GetParameterByName(t, FAILED_OSD_IDS)
	param.Value = strings.Join(ids, ",")
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
		if err := r.Create(r.ctx, cobj, &client.CreateOptions{}); err != nil {
			return err
		}
	}
	return nil
}

// getOSDRemovalPodCompletionStatus
//   - name: Check osd removal job completion
//     shell: "oc get pod -l job-name=ocs-osd-removal-job -n openshift-storage --field-selector=status.phase==\"Succeeded\" -o name"
//     register: result
//     until: 'result.stdout_lines | length > 0'
//     when: 'crash_pods.stdout_lines | length > 0'
//     retries: 30
//     delay: 10
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

// restartStorageOperator
//   - name: Restart storage operator
//     shell: "oc delete pod -n openshift-storage -l app=rook-ceph-operator"
//     register: result
//     when: (crash_pods.stdout_lines | length > 0) or (pending_pods.stdout_lines | length > 0)
//     until: result.rc == 0
//     retries: 30
//     delay: 10
func (r *NodeRecovery) restartStorageOperator() error {
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

func getLatestCondition(instance *odfv1alpha1.NodeRecovery) *odfv1alpha1.RecoveryCondition {
	latest := &instance.Status.Conditions[len(instance.Status.Conditions)-1]
	latest.LastProbeTime = metav1.Now()
	return latest
}

func transitionNextCondition(instance *odfv1alpha1.NodeRecovery, nextCondition odfv1alpha1.RecoveryConditionType) {
	instance.Status.Conditions = append(instance.Status.Conditions, odfv1alpha1.RecoveryCondition{Type: nextCondition, LastTransitionTime: metav1.Now(), LastProbeTime: metav1.Now()})
}

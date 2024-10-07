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
	"github.com/jordigilh/odf-node-recovery-operator/internal/controller/pod"
	odfv1alpha1 "github.com/jordigilh/odf-node-recovery-operator/pkg/api/v1alpha1"
	octemplateapi "github.com/openshift/api/template"
	templatev1client "github.com/openshift/client-go/template/clientset/versioned/typed/template/v1"
	"github.com/openshift/library-go/pkg/template/generator"
	"github.com/openshift/library-go/pkg/template/templateprocessing"
	localv1 "github.com/openshift/local-storage-operator/pkg/common"
	ocsoperatorv1 "github.com/red-hat-storage/ocs-operator/api/v1"
	"github.com/red-hat-storage/ocs-operator/controllers/defaults"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubernetes/pkg/kubelet"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/names"
	"k8s.io/utils/ptr"
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

func newNodeRecoveryReconciler(ctx context.Context, restConfig *rest.Config, scheme *runtime.Scheme, recorder record.EventRecorder, cmdRunner pod.RemoteCommandExecutor) (*NodeRecovery, error) {
	client, err := client.New(restConfig, client.Options{})
	if err != nil {
		return nil, err
	}
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
		r.log.Error(fmt.Errorf("failed to reconcile after retrying for 5 minutes: %s", latestCondition.Message), "Failed to reconcile")
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
		transitionNextCondition(instance, latestCondition, odfv1alpha1.WaitForCephToolsPodRunning)
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
		transitionNextCondition(instance, latestCondition, odfv1alpha1.WaitForOSDPodsStabilize)
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
		transitionNextCondition(instance, latestCondition, odfv1alpha1.LabelNodesWithPendingPods)
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
		transitionNextCondition(instance, latestCondition, odfv1alpha1.ManageCrashLoopBackOffPods)
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
		}
		transitionNextCondition(instance, latestCondition, odfv1alpha1.CleanupOSDRemovalJob)
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
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
		transitionNextCondition(instance, latestCondition, odfv1alpha1.RestartStorageOperator)
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
		transitionNextCondition(instance, latestCondition, odfv1alpha1.DeleteFailedPodsNodeAffinity)
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
		transitionNextCondition(instance, latestCondition, odfv1alpha1.StorageClusterFitnessCheck)
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
		transitionNextCondition(instance, latestCondition, odfv1alpha1.DisableCephTools)
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

func (r *NodeRecovery) getRunningCephToolsPod() (*v1.Pod, error) {
	ctx := r.ctx
	l := &v1.PodList{}
	selectorMap := map[string]string{"app": "rook-ceph-tools"}
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchLabels: selectorMap})
	if err != nil {
		return nil, err
	}
	err = r.List(ctx, l, &client.ListOptions{Namespace: ODF_NAMESPACE, LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	if len(l.Items) == 0 {
		return nil, fmt.Errorf("no pods found with label %v", selectorMap)
	}
	return &l.Items[0], nil
}

// archiveCephDaemonCrashMessages
//   - name: Archive any ceph daemon crash messages
//     shell: "oc rsh -n openshift-storage {{ TOOLS_POD }} ceph crash archive-all"
//     register: result
//     until: result.rc == 0
//     retries: 30
//     delay: 10
func (r *NodeRecovery) archiveCephDaemonCrashMessages() error {
	cmd := []string{"ceph", "crash", "archive-all"}
	pod, err := r.getRunningCephToolsPod()
	if err != nil {
		return err
	}
	_, _, err = r.cmdRunner.Run(pod.Name, pod.Namespace, cmd)
	return err
}

func (r *NodeRecovery) isCephToolsEnabled() (bool, error) {
	o, err := r.getOSCInitialization()
	if err != nil {
		return false, err
	}
	return o.Spec.EnableCephTools, nil
}

// setEnableCephToolsValue configures the ocsinitialization instance in the cluster to enable or disable the deployment of the ceph tools pod
func (r *NodeRecovery) setEnableCephToolsValue(value bool) error {
	o, err := r.getOSCInitialization()
	if err != nil {
		return err
	}
	o.Spec.EnableCephTools = value
	return r.Update(r.ctx, o, &client.UpdateOptions{})
}

func (r NodeRecovery) getOSCInitialization() (*ocsoperatorv1.OCSInitialization, error) {
	ctx := r.ctx
	o := &ocsoperatorv1.OCSInitialization{}
	err := r.Get(ctx, types.NamespacedName{Namespace: ODF_NAMESPACE, Name: "ocsinit"}, o, &client.GetOptions{})
	return o, err
}

func (r *NodeRecovery) enableCephTools() error {
	return r.setEnableCephToolsValue(true)
}

func (r *NodeRecovery) disableCephTools() error {
	return r.setEnableCephToolsValue(false)
}

func (r *NodeRecovery) getCephToolsPodPhase() (v1.PodPhase, error) {
	pod, err := r.getRunningCephToolsPod()
	if err != nil {
		return "", err
	}
	return pod.Status.Phase, nil
}

func (r *NodeRecovery) getPodsInPendingPhase() (*v1.PodList, error) {
	l := &v1.PodList{}
	selectorMap := map[string]string{"app": "rook-ceph-osd"}
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchLabels: selectorMap})
	if err != nil {
		return nil, err
	}
	err = r.List(r.ctx, l, &client.ListOptions{LabelSelector: selector, FieldSelector: fields.ParseSelectorOrDie(`status.phase=="` + string(v1.PodPending) + `"`)})
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
	err := r.List(r.ctx, l, &client.ListOptions{Namespace: ODF_NAMESPACE, FieldSelector: fields.ParseSelectorOrDie(`status.phase=="` + string(v1.PodFailed) + `"`)})
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

// getCephHealthStatus retrieves the health status of Ceph by querying the ceph client in the ceph tools pods
//   - name: Montitor storage cluster returns to HEALTH_OK status
//     shell: "oc rsh -n openshift-storage {{ TOOLS_POD }} ceph status -f json | jq -r '.health.status'"
//     register: result
//     until: result.stdout == "HEALTH_OK"
//     retries: 300
//     delay: 10
func (r *NodeRecovery) getCephHealthStatus() (string, error) {
	p, err := r.getRunningCephToolsPod()
	if err != nil {
		return "", err
	}
	stdout, stderr, err := r.cmdRunner.Run(p.Name, ODF_NAMESPACE, []string{"ceph", "status", "-f", "json", "|", "jq", "-r", "'.health.status'"})
	if err != nil {
		return "", err
	}
	if len(stderr) > 0 {
		return "", fmt.Errorf("error while executing remote shell to retrieve the ceph health status: %s", stderr)
	}
	return stdout, nil
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

// scaleRookCephOSDDeploymentsToZero scales all deployment objects that match the label `app=rook-ceph-osd` to zero
//   - name: Scale to 0
//     shell: "oc scale -n openshift-storage deployment rook-ceph-osd-{{ item | regex_search('rook-ceph-osd-([0-9]+)', '\\1') | first }} --replicas=0"
//     with_items: "{{ crash_pods.stdout_lines }}"
//     when: 'crash_pods.stdout_lines | length > 0'
//     retries: 30
//     delay: 10
func (r *NodeRecovery) scaleRookCephOSDDeploymentsToZero() error {
	l := &appsv1.DeploymentList{}
	selectorMap := map[string]string{"app": "rook-ceph-osd"}
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchLabels: selectorMap})
	if err != nil {
		return err
	}
	err = r.List(r.ctx, l, &client.ListOptions{Namespace: ODF_NAMESPACE, LabelSelector: selector})
	if err != nil {
		return err
	}
	for _, d := range l.Items {
		d.Spec.Replicas = ptr.To(int32(0))
		err = r.Update(r.ctx, &d, &client.UpdateOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

// forceDeleteRookCephOSDPods forces deleting the pods belonging to the Rook Ceph OSD deployments that are stuck and unable to delete after scaling their deployments to zero
// - name: Check if osd pod is still running
//   shell: "oc get -n openshift-storage pods -l ceph-osd-id={{ item | regex_search('rook-ceph-osd-([0-9]+)', '\\1') | first }} -o custom-columns=name:metadata.name --no-headers"
//   register: result
//   with_items: "{{ crash_pods.stdout_lines }}"
//   when: 'crash_pods.stdout_lines | length > 0'
//   retries: 30
//   delay: 10

func (r *NodeRecovery) getRookCephOSDPods() (*v1.PodList, error) {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{Key: "ceph-osd-id", Operator: metav1.LabelSelectorOpExists},
		}},
	)
	if err != nil {
		return nil, err
	}
	l := &v1.PodList{}
	return l, r.List(r.ctx, l, &client.ListOptions{LabelSelector: selector})
}

//   - name: Force delete if still running
//     shell: "oc delete -n openshift-storage pod {{ item.stdout }} --grace-period=0 --force"
//     with_items: "{{ result.results }}"
//     when: 'crash_pods.stdout_lines | length > 0'
//     ignore_errors: true
//     retries: 30
//     delay: 10
func (r *NodeRecovery) forceDeleteRookCephOSDPods() error {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{Key: "ceph-osd-id", Operator: metav1.LabelSelectorOpExists},
		}},
	)
	if err != nil {
		return err
	}
	return r.DeleteAllOf(r.ctx, &v1.Pod{},
		&client.DeleteAllOfOptions{
			ListOptions:   client.ListOptions{Namespace: ODF_NAMESPACE, LabelSelector: selector},
			DeleteOptions: client.DeleteOptions{GracePeriodSeconds: ptr.To(int64(0))},
		})
}

// eraseDevices wipes out the devices associated to the PVs used by the crashing pods
//   - name: Erase devices before adding to OCS cluster
//     shell: "oc debug node/{{ item.name }} --image=registry.redhat.io/rhel8/support-tools -- chroot /host sgdisk --zap-all /dev/{{ item.device }}"
//     register: osd_pods_devices_sgdisk
//     until: osd_pods_devices_sgdisk.rc == 0
//     with_items: "{{ sgdisk }}"
//     retries: 30
//     delay: 10
func (r *NodeRecovery) eraseDevice(nd *nodeDevice) error {
	runner := pod.NewRunner(r.Config)
	podName, nsName, cleanup, err := runner.Initialize(nd.nodeName)
	if err != nil {
		return err
	}
	defer cleanup()
	stdOut, stdErr, err := r.cmdRunner.Run(podName, nsName, []string{"chroot", "/host", "sgdisk", "--zap-all", "/dev/" + nd.deviceName})
	if err != nil {
		return fmt.Errorf("failed to erase disk %s in node %s:%v", nd.deviceName, nd.nodeName, err)
	}
	log.Log.Info("stdout/stderr: %s/%s", stdOut, stdErr)
	return nil
}

// deleteOldOSDRemovalJob
//   - name: Delete old osd removal job
//     shell: "oc delete -n openshift-storage job ocs-osd-removal-job"
//     ignore_errors: true
//     when: 'crash_pods.stdout_lines | length > 0'
//     retries: 30
//     delay: 10
func (r *NodeRecovery) deleteOldOSDRemovalJob() error {
	job := &batchv1.Job{}
	err := r.Get(r.ctx, types.NamespacedName{Namespace: ODF_NAMESPACE, Name: "ocs-osd-removal-job"}, job, &client.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return r.Delete(r.ctx, job, &client.DeleteOptions{})
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

	templateClient, err := templatev1client.NewForConfig(r.Config)
	if err != nil {
		return err
	}
	template, err := templateClient.Templates(ODF_NAMESPACE).Get(r.ctx, "ocs-osd-removal", metav1.GetOptions{})
	if err != nil {
		return err
	}
	param := templateprocessing.GetParameterByName(template, FAILED_OSD_IDS)
	param.Value = strings.Join(ids, ",")
	processor := templateprocessing.NewProcessor(map[string]generator.Generator{
		"expression": generator.NewExpressionValueGenerator(rand.New(rand.NewSource(time.Now().UnixNano()))),
	})
	if errs := processor.Process(template); len(errs) > 0 {
		return kerrors.NewInvalid(octemplateapi.Kind("Template"), template.Name, errs)
	}
	_, err = templateClient.Templates(ODF_NAMESPACE).Create(r.ctx, template, metav1.CreateOptions{})
	return err
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

func transitionNextCondition(instance *odfv1alpha1.NodeRecovery, current *odfv1alpha1.RecoveryCondition, nextCondition odfv1alpha1.RecoveryConditionType) {
	current.Status = v1.ConditionFalse
	instance.Status.Conditions = append(instance.Status.Conditions, odfv1alpha1.RecoveryCondition{Type: nextCondition, Status: v1.ConditionTrue, LastTransitionTime: metav1.Now(), LastProbeTime: metav1.Now()})
}

package controller

import (
	"encoding/json"
	"fmt"

	"github.com/jordigilh/odf-node-recovery-operator/internal/controller/pod"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type healthStatus struct {
	Health health `json:"health"`
}

type health struct {
	Status string `json:"status"`
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

	stdout, stderr, err := r.cmdRunner.Run(p, []string{"ceph", "status", "-f", "json"})
	if err != nil {
		return "", err
	}
	if len(stderr) > 0 {
		return "", fmt.Errorf("error while executing remote shell to retrieve the ceph health status: %s", stderr)
	}
	h := healthStatus{}
	err = json.Unmarshal(stdout, &h)
	if err != nil {
		return "", err
	}
	return h.Health.Status, nil
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
	pod, cleanup, err := runner.Initialize(nd.nodeName)
	if err != nil {
		return err
	}
	defer cleanup()
	stdOut, stdErr, err := r.cmdRunner.Run(pod, []string{"chroot", "/host", "sgdisk", "--zap-all", "/dev/" + nd.deviceName})
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
	_, _, err = r.cmdRunner.Run(pod, cmd)
	return err
}

func (r *NodeRecovery) isCephToolsEnabled() (bool, error) {
	v, err := r.getOCPVersion()
	if err != nil {
		return false, err
	}
	if v.LessThan(ocp4_15) {
		o, err := r.getOSCInitialization()
		if err != nil {
			return false, err
		}
		return o.Spec.EnableCephTools, nil
	}
	sc, err := r.getStorageCluster()
	if err != nil {
		return false, err
	}
	return sc.Spec.EnableCephTools, nil
}

// setEnableCephToolsValue configures the ocsinitialization instance in the cluster to enable or disable the deployment of the ceph tools pod
func (r *NodeRecovery) setEnableCephToolsValue(value bool) error {
	v, err := r.getOCPVersion()
	if err != nil {
		return err
	}
	if v.LessThan(ocp4_15) {
		o, err := r.getOSCInitialization()
		if err != nil {
			return err
		}
		o.Spec.EnableCephTools = value
		return r.Update(r.ctx, o, &client.UpdateOptions{})
	}
	sc, err := r.getStorageCluster()
	if err != nil {
		return err
	}
	sc.Spec.EnableCephTools = value
	return r.Update(r.ctx, sc, &client.UpdateOptions{})
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

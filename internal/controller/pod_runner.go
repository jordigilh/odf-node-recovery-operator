package controller

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	policyapi "k8s.io/pod-security-admission/api"
)

type Runner struct {
	kcli kubernetes.Clientset
}

func NewRunner(config *restclient.Config) (*Runner, error) {
	kcli, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil
	}
	return &Runner{kcli: *kcli}, nil
}

// Initialize creates a temporal namespace with privileged labels and deploys a pod with the node's filesyste mounted in /host
func (c *Runner) Initialize(nodeName string) (string, string, func(), error) {
	nsName, cleanup, err := c.createNamespace()

	if err != nil {
		return "", "", nil, err
	}
	podName, err := c.deployNodeRunnerPod(nsName, nodeName)
	if err != nil {
		return "", "", nil, err
	}

	return podName, nsName, cleanup, nil
}

// createNamespace returns namespace name and clean up function.
// In the manner of node debugging, if default namespace is decided to be used and
// this namespace is not privileged, this function creates temporary namespace.
func (c *Runner) createNamespace() (string, func(), error) {

	tmpNS := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "odf-node-recovery-",
			Labels: map[string]string{
				policyapi.EnforceLevelLabel:                      string(policyapi.LevelPrivileged),
				policyapi.AuditLevelLabel:                        string(policyapi.LevelPrivileged),
				policyapi.WarnLevelLabel:                         string(policyapi.LevelPrivileged),
				"security.openshift.io/scc.podSecurityLabelSync": "false",
			},
			Annotations: map[string]string{
				"openshift.io/node-selector": "",
			},
		},
	}

	ns, err := c.kcli.CoreV1().Namespaces().Create(context.TODO(), tmpNS, metav1.CreateOptions{})
	if err != nil {
		return "", nil, fmt.Errorf("unable to create temporary namespace %s: %v", tmpNS.Name, err)
	}

	cleanup := func() {
		if err := c.kcli.CoreV1().Namespaces().Delete(context.TODO(), ns.Name, metav1.DeleteOptions{}); err != nil {
			klog.V(2).Infof("Unable to delete temporary namespace %s: %v", ns.Name, err)
		}
	}

	return ns.Name, cleanup, nil
}

// deployNodeRunnerPod create a pod that schedules on the specified node.
// The generated pod will run in the host IPC namespaces, and it will have the node's filesystem mounted at /host.
func (c *Runner) deployNodeRunnerPod(namespaceName, nodeName string) (string, error) {
	p := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("node-runner-%s", nodeName),
			Namespace:    namespaceName,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:                     "debugger",
					Image:                    IMAGE_PULLSPEC,
					ImagePullPolicy:          v1.PullIfNotPresent,
					TerminationMessagePolicy: v1.TerminationMessageReadFile,
					VolumeMounts: []v1.VolumeMount{
						{
							MountPath: "/host",
							Name:      "host-root",
						},
					},
					// Command: []string{"chroot", "/host", "sgdisk", "--zap-all", "/dev/" + deviceName},
				},
			},
			HostIPC:       true,
			HostNetwork:   false,
			HostPID:       false,
			NodeName:      nodeName,
			RestartPolicy: v1.RestartPolicyNever,
			Volumes: []v1.Volume{
				{
					Name: "host-root",
					VolumeSource: v1.VolumeSource{
						HostPath: &v1.HostPathVolumeSource{Path: "/"},
					},
				},
			},
			Tolerations: []v1.Toleration{
				{
					Operator: v1.TolerationOpExists,
				},
			},
		},
	}

	p, err := c.kcli.CoreV1().Pods(namespaceName).Create(context.TODO(), p, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("unable to create node runner pod in namespace %s: %v", namespaceName, err)
	}
	return p.Name, nil
}

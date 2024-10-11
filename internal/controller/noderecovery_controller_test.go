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
	"time"

	"github.com/jordigilh/odf-node-recovery-operator/pkg/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	templatev1 "github.com/openshift/api/template/v1"
	localv1 "github.com/openshift/local-storage-operator/pkg/common"
	ocsoperatorv1 "github.com/red-hat-storage/ocs-operator/api/v1"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/apitesting"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubernetes/pkg/kubelet"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/names"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const resourceName = "test-resource"

var _ = Describe("NodeRecovery Controller", func() {
	var (
		controllerReconciler *NodeRecoveryReconciler
		noderecovery         *v1alpha1.NodeRecovery
		// nodeClient           fakeNodeRecovery.Clientset
	)
	Context("When reconciling a resource", func() {
		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name: resourceName,
		}

		BeforeEach(func() {
			By("configuring nodes and OSDInit")

		})
		AfterEach(func() {

		})

		It("should enable the OSD tools pod when not enabled", func() {
			scheme := createFakeScheme()
			os := newNamespace("openshift-storage")
			init := newOCSInit(disabledCephTools)
			nr := getNodeRecovery()
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(os, init, nr).Build()

			Expect(k8sClient).NotTo(BeNil())
			controllerReconciler = &NodeRecoveryReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				Config:    cfg,
				Recorder:  record.NewFakeRecorder(2),
				CmdRunner: newFakeRemoteExecutor("", "", nil),
			}

			By("Reconciling")
			resp, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())
			By("Validating the response")
			Expect(resp.RequeueAfter).To(Equal(10 * time.Second))
			By("Validating the CR status")
			noderecovery = &v1alpha1.NodeRecovery{}
			err = k8sClient.Get(ctx, typeNamespacedName, noderecovery)
			Expect(err).NotTo(HaveOccurred())
			Expect(noderecovery.Status.StartTime.Time.IsZero()).NotTo(BeTrue())
			Expect(noderecovery.Status.CompletionTime).To(BeNil())
			Expect(noderecovery.Status.Conditions).To(HaveLen(2))
			Expect(noderecovery.Status.Conditions[len(noderecovery.Status.Conditions)-1].Type).To(Equal(v1alpha1.WaitForCephToolsPodRunning))
			By("Validating the ocsinit object has the EnableCephTools set to true")
			o := &ocsoperatorv1.OCSInitialization{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: init.Name, Namespace: init.Namespace}, o)).NotTo(HaveOccurred())
			Expect(o.Spec.EnableCephTools).To(BeTrue())
		})

		It("Validating the condition in the status is waiting for the ceph tool to be running", func() {
			scheme := createFakeScheme()
			os := newNamespace("openshift-storage")
			init := newOCSInit(enabledCephTools)
			noderecovery = &v1alpha1.NodeRecovery{
				ObjectMeta: metav1.ObjectMeta{
					Name: resourceName,
				},
				Status: v1alpha1.NodeRecoveryStatus{
					Phase:     v1alpha1.RunningPhase,
					StartTime: &metav1.Time{Time: time.Now()},
					Conditions: []v1alpha1.RecoveryCondition{
						{Type: v1alpha1.WaitForCephToolsPodRunning,
							LastProbeTime:      metav1.NewTime(time.Now()),
							LastTransitionTime: metav1.NewTime(time.Now())},
					},
				},
			}
			p := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ceph-tools",
					Namespace: "openshift-storage",
					Labels:    map[string]string{"app": "rook-ceph-tools"},
				},
				Status: corev1.PodStatus{Phase: corev1.PodPending},
			}
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(os, init, noderecovery, p).Build()
			Expect(k8sClient).NotTo(BeNil())
			controllerReconciler = &NodeRecoveryReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				Config:    cfg,
				Recorder:  record.NewFakeRecorder(2),
				CmdRunner: newFakeRemoteExecutor("", "", nil),
			}
			By("Reconciling the created resource")
			resp, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())
			By("Validating the response")
			Expect(resp.RequeueAfter).To(Equal(10 * time.Second))
			By("Validating the CR status")
			err = k8sClient.Get(ctx, typeNamespacedName, noderecovery)
			Expect(err).NotTo(HaveOccurred())
			Expect(noderecovery.Status.Conditions).To(HaveLen(1))
			Expect(noderecovery.Status.Conditions[len(noderecovery.Status.Conditions)-1].Type).To(Equal(v1alpha1.WaitForCephToolsPodRunning))
			Expect(noderecovery.Status.Conditions[len(noderecovery.Status.Conditions)-1].Reason).To(Equal(v1alpha1.PodNotInRunningPhase))
		})
		It("Validating the condition of the OSD pods to stabilize", func() {
			scheme := createFakeScheme()
			os := newNamespace("openshift-storage")
			init := newOCSInit(enabledCephTools)
			noderecovery = &v1alpha1.NodeRecovery{
				ObjectMeta: metav1.ObjectMeta{
					Name: resourceName,
				},
				Status: v1alpha1.NodeRecoveryStatus{
					Phase:     v1alpha1.RunningPhase,
					StartTime: &metav1.Time{Time: time.Now()},
					Conditions: []v1alpha1.RecoveryCondition{
						{Type: v1alpha1.WaitForCephToolsPodRunning,
							LastProbeTime:      metav1.NewTime(time.Now()),
							LastTransitionTime: metav1.NewTime(time.Now())},
					},
				},
			}
			By("Creating the Ceph Tools pod in running phase")
			p := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ceph-tools",
					Namespace: "openshift-storage",
					Labels:    map[string]string{"app": "rook-ceph-tools"},
				},
				Status: corev1.PodStatus{Phase: corev1.PodRunning},
			}
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(os, init, noderecovery, p).Build()
			Expect(k8sClient).NotTo(BeNil())
			controllerReconciler = &NodeRecoveryReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				Config:    cfg,
				Recorder:  record.NewFakeRecorder(2),
				CmdRunner: newFakeRemoteExecutor("", "", nil),
			}
			By("Reconciling")
			resp, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())
			By("Validating the response")
			Expect(resp.Requeue).To(BeTrue())
			By("Validating the CR status")
			err = k8sClient.Get(ctx, typeNamespacedName, noderecovery)
			Expect(err).NotTo(HaveOccurred())
			Expect(noderecovery.Status.Conditions).To(HaveLen(2))
			Expect(noderecovery.Status.Conditions[len(noderecovery.Status.Conditions)-1].Type).To(Equal(v1alpha1.WaitForOSDPodsStabilize))
		})

		It("Validating the condition of the OSD pods to stabilize", func() {
			scheme := createFakeScheme()
			os := newNamespace("openshift-storage")
			init := newOCSInit(enabledCephTools)
			noderecovery = &v1alpha1.NodeRecovery{
				ObjectMeta: metav1.ObjectMeta{
					Name: resourceName,
				},
				Status: v1alpha1.NodeRecoveryStatus{
					Phase:     v1alpha1.RunningPhase,
					StartTime: &metav1.Time{Time: time.Now()},
					Conditions: []v1alpha1.RecoveryCondition{
						{Type: v1alpha1.WaitForOSDPodsStabilize,
							LastProbeTime:      metav1.NewTime(time.Now()),
							LastTransitionTime: metav1.NewTime(time.Now())},
					},
				},
			}
			By("Creating OSD pods in container creating status")
			p1 := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "initContainerCreating",
					Namespace: "openshift-storage",
				},
				Status: corev1.PodStatus{
					InitContainerStatuses: []corev1.ContainerStatus{
						{State: corev1.ContainerState{
							Waiting: &corev1.ContainerStateWaiting{
								Reason: kubelet.ContainerCreating}}}},
				},
			}
			p2 := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "podInitializing",
					Namespace: "openshift-storage",
				},
				Status: corev1.PodStatus{
					InitContainerStatuses: []corev1.ContainerStatus{
						{Name: "running",
							State: corev1.ContainerState{
								Running: &corev1.ContainerStateRunning{StartedAt: metav1.Now()}}}},
					ContainerStatuses: []corev1.ContainerStatus{
						{Name: "containerCreating",
							State: corev1.ContainerState{
								Waiting: &corev1.ContainerStateWaiting{
									Reason: kubelet.PodInitializing,
								},
							}},
					},
				},
			}
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(os, init, noderecovery, p1, p2).Build()
			Expect(k8sClient).NotTo(BeNil())
			controllerReconciler = &NodeRecoveryReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				Config:    cfg,
				Recorder:  record.NewFakeRecorder(2),
				CmdRunner: newFakeRemoteExecutor("", "", nil),
			}
			By("Reconciling")
			resp, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())
			By("Validating the response")
			Expect(resp.RequeueAfter).To(Equal(10 * time.Second))
			By("Validating the CR status")
			err = k8sClient.Get(ctx, typeNamespacedName, noderecovery)
			Expect(err).NotTo(HaveOccurred())
			Expect(noderecovery.Status.Conditions).To(HaveLen(1))
			Expect(noderecovery.Status.Conditions[len(noderecovery.Status.Conditions)-1].Type).To(Equal(v1alpha1.WaitForOSDPodsStabilize))
			Expect(noderecovery.Status.Conditions[len(noderecovery.Status.Conditions)-1].Reason).To(Equal(v1alpha1.WaitingForPodsToInitialize))
		})

		It("Validating the transition from waiting for OSD pods to initialize to checking for pods in pending status", func() {
			By("Creating the CR")
			scheme := createFakeScheme()
			os := newNamespace("openshift-storage")
			init := newOCSInit(enabledCephTools)
			noderecovery = &v1alpha1.NodeRecovery{
				ObjectMeta: metav1.ObjectMeta{
					Name: resourceName,
				},
				Status: v1alpha1.NodeRecoveryStatus{
					Phase:     v1alpha1.RunningPhase,
					StartTime: &metav1.Time{Time: time.Now()},
					Conditions: []v1alpha1.RecoveryCondition{
						{Type: v1alpha1.WaitForOSDPodsStabilize,
							LastProbeTime:      metav1.NewTime(time.Now()),
							LastTransitionTime: metav1.NewTime(time.Now())},
					},
				},
			}

			By("Creating a pod in pending phase")
			p1 := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pending",
					Namespace: "openshift-storage",
					Labels:    map[string]string{"app": "rook-ceph-osd"},
				},
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{corev1.LabelHostname: "foo"},
				},
				Status: corev1.PodStatus{Phase: corev1.PodPending},
			}

			By("Creating the node where the pod is running")
			n := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
			}
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(os, init, noderecovery, p1, n).WithIndex(&v1.Pod{}, "status.phase", filterByPhase).Build()
			Expect(k8sClient).NotTo(BeNil())
			controllerReconciler = &NodeRecoveryReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				Config:    cfg,
				Recorder:  record.NewFakeRecorder(2),
				CmdRunner: newFakeRemoteExecutor("", "", nil),
			}

			By("Reconciling first to validate no pods are in creating status")
			resp, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Validating the response")
			Expect(resp.Requeue).To(BeTrue())

			By("Validating the CR status")
			err = k8sClient.Get(ctx, typeNamespacedName, noderecovery)
			Expect(err).NotTo(HaveOccurred())
			Expect(noderecovery.Status.Conditions).To(HaveLen(2))
			Expect(noderecovery.Status.PendingPods).To(BeFalse())
			Expect(noderecovery.Status.Conditions[len(noderecovery.Status.Conditions)-1].Type).To(Equal(v1alpha1.LabelNodesWithPendingPods))
			Expect(noderecovery.Status.Conditions[len(noderecovery.Status.Conditions)-1].Reason).To(BeEmpty())

			By("Reconciling again to check for pending pods")
			resp, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Validating the response")
			Expect(resp.Requeue).To(BeTrue())

			By("Validating the CR status")
			err = k8sClient.Get(ctx, typeNamespacedName, noderecovery)
			Expect(err).NotTo(HaveOccurred())
			Expect(noderecovery.Status.Conditions).To(HaveLen(3))
			Expect(noderecovery.Status.PendingPods).To(BeTrue())
			Expect(noderecovery.Status.Conditions[len(noderecovery.Status.Conditions)-1].Type).To(Equal(v1alpha1.ManageCrashLoopBackOffPods))
			Expect(noderecovery.Status.Conditions[len(noderecovery.Status.Conditions)-1].Reason).To(BeEmpty())
		})

		It("Validating the condition of managing pods in crashloopbackoff status", func() {
			By("Creating the CR")

			scheme := createFakeScheme()
			os := newNamespace("openshift-storage")
			init := newOCSInit(enabledCephTools)
			template := newTemplate()
			noderecovery = &v1alpha1.NodeRecovery{
				ObjectMeta: metav1.ObjectMeta{
					Name: resourceName,
				},
				Status: v1alpha1.NodeRecoveryStatus{
					Phase:     v1alpha1.RunningPhase,
					StartTime: &metav1.Time{Time: time.Now()},
					Conditions: []v1alpha1.RecoveryCondition{
						{Type: v1alpha1.ManageCrashLoopBackOffPods,
							LastProbeTime:      metav1.NewTime(time.Now()),
							LastTransitionTime: metav1.NewTime(time.Now())},
					},
				},
			}

			By("Creating OSD pods and PV/PVCs in crashloopback status")
			p1 := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pv-pod",
					Namespace: "openshift-storage",
					Labels: map[string]string{
						osd.OSDOverPVCLabelKey: "pvcName",
						"app":                  "rook-ceph-osd",
						osd.OsdIdLabelKey:      "1",
					},
				},
				Status: corev1.PodStatus{
					InitContainerStatuses: []corev1.ContainerStatus{
						{State: corev1.ContainerState{
							Waiting: &corev1.ContainerStateWaiting{
								Reason: "CrashLoopBackOff"}}}},
				},
			}

			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pvcName",
					Namespace: "openshift-storage",
					Labels:    map[string]string{corev1.LabelHostname: "foo"},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					VolumeName: "pvName",
				},
			}
			pv := &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pvName",
					Annotations: map[string]string{
						localv1.PVDeviceNameLabel: "vdb",
					},
				},
			}

			By("Creating Ceph OSD Deployments")
			d := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "openshift-storage",
					Labels: map[string]string{
						"app": "rook-ceph-osd",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To(int32(1)),
				},
			}
			By("Creating pods with ceph-osd-id label")
			p2 := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ceph-osd-id",
					Namespace: "openshift-storage",
					Labels: map[string]string{
						"ceph-osd-id": "1",
					},
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
					Finalizers:        []string{"foo"},
				},
			}
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(os, init, noderecovery, template, d, p1, p2, pv, pvc).WithIndex(&v1.Pod{}, "status.phase", filterByPhase).Build()
			Expect(k8sClient).NotTo(BeNil())
			controllerReconciler = &NodeRecoveryReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				Config:    cfg,
				Recorder:  record.NewFakeRecorder(2),
				CmdRunner: newFakeRemoteExecutor("", "", nil),
			}

			By("Reconciling the created resource")
			resp, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Validating the response")
			Expect(resp.RequeueAfter).To(Equal(15 * time.Second))

			By("Validating the CR status")
			err = k8sClient.Get(ctx, typeNamespacedName, noderecovery)
			Expect(err).NotTo(HaveOccurred())
			Expect(noderecovery.Status.Conditions).To(HaveLen(2))
			Expect(noderecovery.Status.CrashLoopBackOffPods).To(BeTrue())
			Expect(noderecovery.Status.Conditions[len(noderecovery.Status.Conditions)-1].Type).To(Equal(v1alpha1.CleanupOSDRemovalJob))
			Expect(noderecovery.Status.Conditions[len(noderecovery.Status.Conditions)-1].Reason).To(BeEmpty())
		})

		It("Validating the condition of cleaning up the osd removal job status when not yet succeeded", func() {
			scheme := createFakeScheme()
			os := newNamespace("openshift-storage")
			init := newOCSInit(enabledCephTools)
			noderecovery := &v1alpha1.NodeRecovery{
				ObjectMeta: metav1.ObjectMeta{
					Name: resourceName,
				},
				Status: v1alpha1.NodeRecoveryStatus{
					Phase:     v1alpha1.RunningPhase,
					StartTime: &metav1.Time{Time: time.Now()},
					Conditions: []v1alpha1.RecoveryCondition{
						{Type: v1alpha1.CleanupOSDRemovalJob,
							LastProbeTime:      metav1.NewTime(time.Now()),
							LastTransitionTime: metav1.NewTime(time.Now())},
					},
				},
			}

			By("Creating the osd-removal-job pod in running phase")
			p1 := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ocs-osd-removal-job",
					Namespace: "openshift-storage",
					Labels: map[string]string{
						"job-name": "ocs-osd-removal-job",
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			}

			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(os, init, noderecovery, p1).Build()
			Expect(k8sClient).NotTo(BeNil())
			controllerReconciler = &NodeRecoveryReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				Config:    cfg,
				Recorder:  record.NewFakeRecorder(2),
				CmdRunner: newFakeRemoteExecutor("", "", nil),
			}

			By("Reconciling the created resource")
			resp, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Validating the response")
			Expect(resp.RequeueAfter).To(Equal(10 * time.Second))

			By("Validating the CR status")
			err = k8sClient.Get(ctx, typeNamespacedName, noderecovery)
			Expect(err).NotTo(HaveOccurred())
			Expect(noderecovery.Status.Conditions).To(HaveLen(1))
			Expect(noderecovery.Status.Conditions[len(noderecovery.Status.Conditions)-1].Type).To(Equal(v1alpha1.CleanupOSDRemovalJob))
			Expect(noderecovery.Status.Conditions[len(noderecovery.Status.Conditions)-1].Reason).To(BeEmpty())
		})

		It("Validating the condition of cleaning up the osd removal job status when succeeded", func() {
			scheme := createFakeScheme()
			os := newNamespace("openshift-storage")
			init := newOCSInit(enabledCephTools)
			noderecovery := &v1alpha1.NodeRecovery{
				ObjectMeta: metav1.ObjectMeta{
					Name: resourceName,
				},
				Status: v1alpha1.NodeRecoveryStatus{
					Phase:     v1alpha1.RunningPhase,
					StartTime: &metav1.Time{Time: time.Now()},
					Conditions: []v1alpha1.RecoveryCondition{
						{Type: v1alpha1.CleanupOSDRemovalJob,
							LastProbeTime:      metav1.NewTime(time.Now()),
							LastTransitionTime: metav1.NewTime(time.Now())},
					},
				},
			}
			By("Validating the osd-removal-job pod in succeeded phase")
			p1 := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ocs-osd-removal-job",
					Namespace: "openshift-storage",
					Labels: map[string]string{
						"job-name": "ocs-osd-removal-job",
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodSucceeded,
				},
			}

			By("Creating the osd-removal-job ")
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ocs-osd-removal-job",
					Namespace: "openshift-storage",
				},
			}

			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(os, init, noderecovery, p1, job).Build()
			Expect(k8sClient).NotTo(BeNil())
			controllerReconciler = &NodeRecoveryReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				Config:    cfg,
				Recorder:  record.NewFakeRecorder(2),
				CmdRunner: newFakeRemoteExecutor("", "", nil),
			}

			By("Reconciling the created resource")
			resp, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Validating the response")
			Expect(resp.Requeue).To(BeTrue())

			By("Validating the CR status")
			err = k8sClient.Get(ctx, typeNamespacedName, noderecovery)
			Expect(err).NotTo(HaveOccurred())
			Expect(noderecovery.Status.Conditions).To(HaveLen(2))
			Expect(noderecovery.Status.Conditions[len(noderecovery.Status.Conditions)-1].Type).To(Equal(v1alpha1.RestartStorageOperator))
			Expect(noderecovery.Status.Conditions[len(noderecovery.Status.Conditions)-1].Reason).To(BeEmpty())
			By("Validating the osd-removal-job pod has been deleted")
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: job.Namespace, Name: job.Name}, job)
			Expect(kerrors.IsNotFound(err)).To(BeTrue())
		})

		It("Validating the condition of restarting the storage operator when pending pods were found", func() {
			scheme := createFakeScheme()
			os := newNamespace("openshift-storage")
			init := newOCSInit(enabledCephTools)
			noderecovery := &v1alpha1.NodeRecovery{
				ObjectMeta: metav1.ObjectMeta{
					Name: resourceName,
				},
				Status: v1alpha1.NodeRecoveryStatus{
					Phase:       v1alpha1.RunningPhase,
					StartTime:   &metav1.Time{Time: time.Now()},
					PendingPods: true,
					Conditions: []v1alpha1.RecoveryCondition{
						{Type: v1alpha1.RestartStorageOperator,
							LastProbeTime:      metav1.NewTime(time.Now()),
							LastTransitionTime: metav1.NewTime(time.Now())},
					},
				},
			}

			By("Creating a storage operator pod")
			p := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "storage-operator",
					Namespace: "openshift-storage",
					Labels: map[string]string{
						"app": "rook-ceph-operator",
					},
				},
			}

			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(os, init, noderecovery, p).Build()
			Expect(k8sClient).NotTo(BeNil())
			controllerReconciler = &NodeRecoveryReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				Config:    cfg,
				Recorder:  record.NewFakeRecorder(2),
				CmdRunner: newFakeRemoteExecutor("", "", nil),
			}

			By("Reconciling the CR")
			resp, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Validating the response")
			Expect(resp.Requeue).To(BeTrue())

			By("Validating the CR status")
			err = k8sClient.Get(ctx, typeNamespacedName, noderecovery)
			Expect(err).NotTo(HaveOccurred())
			Expect(noderecovery.Status.Conditions).To(HaveLen(2))
			Expect(noderecovery.Status.Conditions[len(noderecovery.Status.Conditions)-1].Type).To(Equal(v1alpha1.DeleteFailedPodsNodeAffinity))
		})

		It("Validating the condition of restarting the storage operator when pods with crashloopbackoff status were found", func() {
			scheme := createFakeScheme()
			os := newNamespace("openshift-storage")
			init := newOCSInit(enabledCephTools)
			noderecovery := &v1alpha1.NodeRecovery{
				ObjectMeta: metav1.ObjectMeta{
					Name: resourceName,
				},
				Status: v1alpha1.NodeRecoveryStatus{
					Phase:                v1alpha1.RunningPhase,
					StartTime:            &metav1.Time{Time: time.Now()},
					CrashLoopBackOffPods: true,
					Conditions: []v1alpha1.RecoveryCondition{
						{Type: v1alpha1.RestartStorageOperator,
							LastProbeTime:      metav1.NewTime(time.Now()),
							LastTransitionTime: metav1.NewTime(time.Now())},
					},
				},
			}

			By("Creating a storage operator pod")
			p := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "storage-operator",
					Namespace: "openshift-storage",
					Labels: map[string]string{
						"app": "rook-ceph-operator",
					},
				},
			}

			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(os, init, noderecovery, p).Build()
			Expect(k8sClient).NotTo(BeNil())
			controllerReconciler = &NodeRecoveryReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				Config:    cfg,
				Recorder:  record.NewFakeRecorder(2),
				CmdRunner: newFakeRemoteExecutor("", "", nil),
			}

			By("Reconciling the CR")
			resp, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Validating the response")
			Expect(resp.Requeue).To(BeTrue())

			By("Validating the CR status")
			err = k8sClient.Get(ctx, typeNamespacedName, noderecovery)
			Expect(err).NotTo(HaveOccurred())
			Expect(noderecovery.Status.Conditions).To(HaveLen(2))
			Expect(noderecovery.Status.Conditions[len(noderecovery.Status.Conditions)-1].Type).To(Equal(v1alpha1.DeleteFailedPodsNodeAffinity))
		})

		It("Validating the condition of deleting the failed pods due to node affinity", func() {
			scheme := createFakeScheme()
			os := newNamespace("openshift-storage")
			init := newOCSInit(enabledCephTools)
			noderecovery := &v1alpha1.NodeRecovery{
				ObjectMeta: metav1.ObjectMeta{
					Name: resourceName,
				},
				Status: v1alpha1.NodeRecoveryStatus{
					Phase:                v1alpha1.RunningPhase,
					StartTime:            &metav1.Time{Time: time.Now()},
					CrashLoopBackOffPods: true,
					Conditions: []v1alpha1.RecoveryCondition{
						{Type: v1alpha1.DeleteFailedPodsNodeAffinity,
							LastProbeTime:      metav1.NewTime(time.Now()),
							LastTransitionTime: metav1.NewTime(time.Now())},
					},
				},
			}

			By("Creating a failed pod with reason node affinity")
			p := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "failed-pod",
					Namespace: "openshift-storage",
				},
				Status: corev1.PodStatus{
					Phase:  corev1.PodFailed,
					Reason: names.NodeAffinity,
				},
			}

			tools := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "storage-operator",
					Namespace: "openshift-storage",
					Labels: map[string]string{
						"app": "rook-ceph-tools",
					},
				},
			}

			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(os, init, noderecovery, tools, p).WithIndex(&v1.Pod{}, "status.phase", filterByPhase).Build()
			Expect(k8sClient).NotTo(BeNil())
			controllerReconciler = &NodeRecoveryReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				Config:    cfg,
				Recorder:  record.NewFakeRecorder(2),
				CmdRunner: newFakeRemoteExecutor("", "", nil),
			}

			By("Reconciling the CR")
			resp, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Validating the response")
			Expect(resp.Requeue).To(BeTrue())

			By("Validating the CR status")
			err = k8sClient.Get(ctx, typeNamespacedName, noderecovery)
			Expect(err).NotTo(HaveOccurred())
			Expect(noderecovery.Status.Conditions).To(HaveLen(2))
			Expect(noderecovery.Status.Conditions[len(noderecovery.Status.Conditions)-1].Type).To(Equal(v1alpha1.StorageClusterFitnessCheck))
		})

		It("Validating the condition of storage cluster fitness check", func() {
			scheme := createFakeScheme()
			os := newNamespace("openshift-storage")
			init := newOCSInit(enabledCephTools)
			noderecovery := &v1alpha1.NodeRecovery{
				ObjectMeta: metav1.ObjectMeta{
					Name: resourceName,
				},
				Status: v1alpha1.NodeRecoveryStatus{
					Phase:                v1alpha1.RunningPhase,
					StartTime:            &metav1.Time{Time: time.Now()},
					CrashLoopBackOffPods: true,
					Conditions: []v1alpha1.RecoveryCondition{
						{Type: v1alpha1.StorageClusterFitnessCheck,
							LastProbeTime:      metav1.NewTime(time.Now()),
							LastTransitionTime: metav1.NewTime(time.Now())},
					},
				},
			}
			By("Creating a storage operator pod")
			p := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "storage-operator",
					Namespace: "openshift-storage",
					Labels: map[string]string{
						"app": "rook-ceph-tools",
					},
				},
			}
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(os, init, noderecovery, p).Build()
			Expect(k8sClient).NotTo(BeNil())
			controllerReconciler = &NodeRecoveryReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				Config:    cfg,
				Recorder:  record.NewFakeRecorder(2),
				CmdRunner: newFakeRemoteExecutor("HEALTH_OK", "", nil),
			}

			By("Reconciling the CR")
			resp, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Validating the response")
			Expect(resp.Requeue).To(BeTrue())

			By("Validating the CR status")
			err = k8sClient.Get(ctx, typeNamespacedName, noderecovery)
			Expect(err).NotTo(HaveOccurred())
			Expect(noderecovery.Status.Conditions).To(HaveLen(2))
			Expect(noderecovery.Status.Conditions[len(noderecovery.Status.Conditions)-1].Type).To(Equal(v1alpha1.DisableCephTools))
		})

		It("Validating the condition of disabling the ceph tools pod", func() {
			scheme := createFakeScheme()
			os := newNamespace("openshift-storage")
			init := newOCSInit(enabledCephTools)
			noderecovery := &v1alpha1.NodeRecovery{
				ObjectMeta: metav1.ObjectMeta{
					Name: resourceName,
				},
				Status: v1alpha1.NodeRecoveryStatus{
					Phase:     v1alpha1.RunningPhase,
					StartTime: &metav1.Time{Time: time.Now()},
					Conditions: []v1alpha1.RecoveryCondition{
						{Type: v1alpha1.DisableCephTools,
							LastProbeTime:      metav1.NewTime(time.Now()),
							LastTransitionTime: metav1.NewTime(time.Now())},
					},
				},
			}

			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(os, init, noderecovery).Build()
			Expect(k8sClient).NotTo(BeNil())
			controllerReconciler = &NodeRecoveryReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				Config:    cfg,
				Recorder:  record.NewFakeRecorder(2),
				CmdRunner: newFakeRemoteExecutor("", "", nil),
			}

			By("Reconciling the CR")
			resp, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())
			By("Validating the response")
			Expect(resp).To(Equal(reconcile.Result{}))

			By("Validating the CR status")
			err = k8sClient.Get(ctx, typeNamespacedName, noderecovery)
			Expect(err).NotTo(HaveOccurred())
			Expect(noderecovery.Status.Phase).To(Equal(v1alpha1.CompletedPhase))
			Expect(noderecovery.Status.CompletionTime.Time.IsZero()).To(BeFalse())
			Expect(noderecovery.Status.Conditions).To(HaveLen(1))
		})

	})
})

type cephToolsState bool

const (
	enabledCephTools  cephToolsState = true
	disabledCephTools cephToolsState = false
)

type fakeRemoteExecutor struct {
	stdout, stderr string
	err            error
}

func newFakeRemoteExecutor(stdout, stderr string, err error) *fakeRemoteExecutor {
	return &fakeRemoteExecutor{stdout: stdout, stderr: stderr, err: err}
}

func (f *fakeRemoteExecutor) Run(podName, namespaceName string, cmd []string) (string, string, error) {
	return f.stdout, f.stderr, f.err
}

func newNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
}

func newOCSInit(enabledTools cephToolsState) *ocsoperatorv1.OCSInitialization {
	return &ocsoperatorv1.OCSInitialization{ObjectMeta: metav1.ObjectMeta{Name: "ocsinit", Namespace: "openshift-storage"}, Spec: ocsoperatorv1.OCSInitializationSpec{EnableCephTools: bool(enabledTools)}}
}

func getNodeRecovery() *v1alpha1.NodeRecovery {
	return &v1alpha1.NodeRecovery{ObjectMeta: metav1.ObjectMeta{Name: resourceName}}
}

func filterByPhase(obj client.Object) []string {
	return []string{string(obj.(*corev1.Pod).Status.Phase)}
}

func newTemplate() *templatev1.Template {
	var template templatev1.Template
	_, codecFactory := apitesting.SchemeForOrDie(templatev1.Install)
	decoder := codecFactory.UniversalDecoder()
	err := runtime.DecodeInto(decoder, []byte(`{
  "kind": "Template",
  "apiVersion": "template.openshift.io/v1",
  "metadata": {
    "name": "`+OCS_OSD_REMOVAL+`",
    "namespace": "`+ODF_NAMESPACE+`"
  },
  "objects": [
    {
      "kind": "Service",
      "apiVersion": "v1",
      "metadata": {
	    "name": "serviceA",
		"namespace": "openshift-storage",
        "labels": {
          "failed_osd_ids": "${FAILED_OSD_IDS}"
        }
      }
    }
  ],
  "parameters": [
    {
      "name": "FAILED_OSD_IDS",
      "required": true
    }
  ]
}`), &template)
	Expect(err).NotTo(HaveOccurred())
	return &template
}

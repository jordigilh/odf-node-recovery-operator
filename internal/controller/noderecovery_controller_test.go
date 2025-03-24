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
	"github.com/red-hat-storage/ocs-operator/controllers/defaults"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	configv1 "github.com/openshift/api/config/v1"
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

const (
	resourceName   = "test-resource"
	oscinitVersion = "v4.13.0"
)

var _ = Describe("NodeRecovery Controller", func() {
	var (
		controllerReconciler *NodeRecoveryReconciler
		nodeRecovery         *v1alpha1.NodeRecovery
		fakeClientBuilder    *fake.ClientBuilder
		scheme               = createFakeScheme()
		os                   = newNamespace("openshift-storage")
		version              = newOCPVersion(oscinitVersion)
	)
	Context("When reconciling a resource", func() {
		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name: resourceName,
		}

		BeforeEach(func() {
			By("configuring nodes and OSDInit")
			fakeClientBuilder = fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(version, os).WithStatusSubresource(&v1alpha1.NodeRecovery{}).WithIndex(&corev1.Pod{}, podStatusPhaseFieldSelector, filterByPhase)
		})
		AfterEach(func() {

		})

		It("should enable the OSD tools pod when not enabled", func() {
			init := newOCSInit(disabledCephTools)
			k8sClient = fakeClientBuilder.WithRuntimeObjects(init, getNodeRecovery()).Build()
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
			nodeRecovery = &v1alpha1.NodeRecovery{}
			err = k8sClient.Get(ctx, typeNamespacedName, nodeRecovery)
			Expect(err).NotTo(HaveOccurred())
			Expect(nodeRecovery.Status.StartTime.Time.IsZero()).NotTo(BeTrue())
			Expect(nodeRecovery.Status.CompletionTime).To(BeNil())
			validateConditions(nodeRecovery, 2, v1alpha1.WaitForCephToolsPodRunning, "")
			By("Validating the ocsinit object has the EnableCephTools set to true")
			o := &ocsoperatorv1.OCSInitialization{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: init.Name, Namespace: init.Namespace}, o)).NotTo(HaveOccurred())
			Expect(o.Spec.EnableCephTools).To(BeTrue())
		})

		It("should flag the KeepCephToolsPod as true when the pod already exists", func() {
			init := newOCSInit(enabledCephTools)
			k8sClient = fakeClientBuilder.WithRuntimeObjects(init, getNodeRecovery()).Build()
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
			nodeRecovery = &v1alpha1.NodeRecovery{}
			err = k8sClient.Get(ctx, typeNamespacedName, nodeRecovery)
			Expect(err).NotTo(HaveOccurred())
			Expect(nodeRecovery.Status.StartTime.Time.IsZero()).NotTo(BeTrue())
			Expect(nodeRecovery.Status.CompletionTime).To(BeNil())
			Expect(nodeRecovery.Status.KeepCephToolsPod).To(BeTrue())
			validateConditions(nodeRecovery, 2, v1alpha1.WaitForCephToolsPodRunning, "")
			By("Validating the ocsinit object has the EnableCephTools set to true")
			o := &ocsoperatorv1.OCSInitialization{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: init.Name, Namespace: init.Namespace}, o)).NotTo(HaveOccurred())
			Expect(o.Spec.EnableCephTools).To(BeTrue())
		})

		It("Validates the condition in the status is waiting for the ceph tool to be running", func() {
			init := newOCSInit(enabledCephTools)
			nodeRecovery = getNodeRecoveryWithStatus(v1alpha1.WaitForCephToolsPodRunning)
			p := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ceph-tools",
					Namespace: "openshift-storage",
					Labels:    map[string]string{"app": "rook-ceph-tools"},
				},
				Status: corev1.PodStatus{Phase: corev1.PodPending},
			}
			k8sClient = fakeClientBuilder.WithRuntimeObjects(init, nodeRecovery, p).Build()
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
			err = k8sClient.Get(ctx, typeNamespacedName, nodeRecovery)
			Expect(err).NotTo(HaveOccurred())
			validateConditions(nodeRecovery, 1, v1alpha1.WaitForCephToolsPodRunning, v1alpha1.PodNotInRunningPhase)
		})
		It("Validates the condition of the OSD pods to stabilize", func() {
			init := newOCSInit(enabledCephTools)
			nodeRecovery = getNodeRecoveryWithStatus(v1alpha1.WaitForCephToolsPodRunning)
			By("Creating the Ceph Tools pod in running phase")
			p := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ceph-tools",
					Namespace: "openshift-storage",
					Labels:    map[string]string{"app": "rook-ceph-tools"},
				},
				Status: corev1.PodStatus{Phase: corev1.PodRunning},
			}
			k8sClient = fakeClientBuilder.WithRuntimeObjects(init, nodeRecovery, p).Build()
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
			err = k8sClient.Get(ctx, typeNamespacedName, nodeRecovery)
			Expect(err).NotTo(HaveOccurred())
			validateConditions(nodeRecovery, 2, v1alpha1.WaitForOSDPodsStabilize, "")
		})

		It("Validates the condition of the OSD pods to stabilize", func() {
			init := newOCSInit(enabledCephTools)
			nodeRecovery = getNodeRecoveryWithStatus(v1alpha1.WaitForOSDPodsStabilize)

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
			k8sClient = fakeClientBuilder.WithRuntimeObjects(init, nodeRecovery, p1, p2).Build()
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
			err = k8sClient.Get(ctx, typeNamespacedName, nodeRecovery)
			Expect(err).NotTo(HaveOccurred())
			validateConditions(nodeRecovery, 1, v1alpha1.WaitForOSDPodsStabilize, v1alpha1.WaitingForPodsToInitialize)
		})

		It("Validates the transition from waiting for OSD pods to initialize to checking for pods in pending status", func() {
			By("Creating the CR")
			nodeRecovery = getNodeRecoveryWithStatus(v1alpha1.WaitForOSDPodsStabilize)
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
			k8sClient = fakeClientBuilder.WithRuntimeObjects(nodeRecovery, p1, n).Build()
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
			err = k8sClient.Get(ctx, typeNamespacedName, nodeRecovery)
			Expect(err).NotTo(HaveOccurred())
			validateConditions(nodeRecovery, 2, v1alpha1.LabelNodesWithPendingPods, "")
			Expect(nodeRecovery.Status.PendingPods).To(BeFalse())
		})
		It("Validates the transition from label nodes with pending pods pods to Manage crashloopbackoff pods", func() {
			By("Creating the CR")
			nodeRecovery = getNodeRecoveryWithStatus(v1alpha1.LabelNodesWithPendingPods)
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
			k8sClient = fakeClientBuilder.WithRuntimeObjects(nodeRecovery, p1, n).Build()
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
			err = k8sClient.Get(ctx, typeNamespacedName, nodeRecovery)
			Expect(err).NotTo(HaveOccurred())
			Expect(nodeRecovery.Status.PendingPods).To(BeTrue())
			validateConditions(nodeRecovery, 2, v1alpha1.ManageCrashLoopBackOffPods, "")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: n.Name}, n)
			Expect(err).NotTo(HaveOccurred())
			Expect(n.Labels).To(BeEquivalentTo(map[string]string{defaults.NodeAffinityKey: ""}))
		})

		It("Validates the condition of managing pods in crashloopbackoff status", func() {
			By("Creating the CR")
			template := newTemplate()
			nodeRecovery = getNodeRecoveryWithStatus(v1alpha1.ManageCrashLoopBackOffPods)
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
			k8sClient = fakeClientBuilder.WithRuntimeObjects(nodeRecovery, template, d, p1, p2, pv, pvc).Build()
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
			err = k8sClient.Get(ctx, typeNamespacedName, nodeRecovery)
			Expect(err).NotTo(HaveOccurred())
			Expect(nodeRecovery.Status.CrashLoopBackOffPods).To(BeTrue())
			validateConditions(nodeRecovery, 2, v1alpha1.ForceDeleteRookCephOSDPods, "")
		})

		It("Validates the condition where no pods are in CrashBackLookBackOff", func() {
			By("Creating the CR")
			nodeRecovery = getNodeRecoveryWithStatus(v1alpha1.ManageCrashLoopBackOffPods)
			k8sClient = fakeClientBuilder.WithRuntimeObjects(nodeRecovery).Build()
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
			err = k8sClient.Get(ctx, typeNamespacedName, nodeRecovery)
			Expect(err).NotTo(HaveOccurred())
			validateConditions(nodeRecovery, 2, v1alpha1.RestartStorageOperator, "")
		})

		It("Validates the condition where force deleting the OSD pods through the OSD removal job when there are >3 OSD instances", func() {
			By("Creating the CR")
			template := newTemplate()
			nodeRecovery = getNodeRecoveryWithStatus(v1alpha1.ForceDeleteRookCephOSDPods)
			nodeRecovery.Status.CrashLoopBackOffPods = true
			nodeRecovery.Status.CrashedOSDDeploymentIDs = []string{"1"}
			By("Creating OSD pods and PV/PVCs in crashloopback status")
			p1 := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					DeletionTimestamp: &metav1.Time{Time: time.Now().Add(-2 * time.Minute)},
					Finalizers:        []string{"foo"},
					Name:              "pv-pod-1",
					Namespace:         "openshift-storage",
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

			p2 := generateOSDPodObject("2")
			p3 := generateOSDPodObject("3")
			p4 := generateOSDPodObject("4")

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
			osdPod := &corev1.Pod{
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
			k8sClient = fakeClientBuilder.WithRuntimeObjects(nodeRecovery, template, d, p1, p2, p3, p4, osdPod, pv, pvc).Build()
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

			Expect(nodeRecovery.Status.CrashLoopBackOffPods).To(BeTrue())
			validateConditions(nodeRecovery, 1, v1alpha1.ForceDeleteRookCephOSDPods, "")

		})

		It("Validates the condition in force delete the OSD pods when there are none left and it transitions to the next phase", func() {
			By("Creating the CR")
			template := newTemplate()
			nodeRecovery = getNodeRecoveryWithStatus(v1alpha1.ForceDeleteRookCephOSDPods)
			nodeRecovery.Status.CrashLoopBackOffPods = true
			nodeRecovery.Status.CrashedOSDDeploymentIDs = []string{"1"}

			k8sClient = fakeClientBuilder.WithRuntimeObjects(nodeRecovery, template).Build()
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
			err = k8sClient.Get(ctx, typeNamespacedName, nodeRecovery)
			Expect(err).NotTo(HaveOccurred())
			Expect(nodeRecovery.Status.CrashLoopBackOffPods).To(BeTrue())
			validateConditions(nodeRecovery, 2, v1alpha1.ProcessOCSRemovalTemplate, "")
		})

		It("Validates the condition after successfully deleting the OSD pods and processing the OSD Removal Template", func() {
			By("Creating the CR")
			template := newTemplate()
			nodeRecovery = getNodeRecoveryWithStatus(v1alpha1.ProcessOCSRemovalTemplate)
			nodeRecovery.Status.CrashLoopBackOffPods = true
			nodeRecovery.Status.CrashedOSDDeploymentIDs = []string{"1"}
			k8sClient = fakeClientBuilder.WithRuntimeObjects(nodeRecovery, template).Build()
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

			By("Validating the arguments in the template")
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "noderec-configmap", Namespace: ODF_NAMESPACE}, cm)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Data[FAILED_OSD_IDS]).To(Equal("1"))
			Expect(cm.Data[FORCE_OSD_REMOVAL]).To(Equal("true"))

			By("Validating the CR status")
			err = k8sClient.Get(ctx, typeNamespacedName, nodeRecovery)
			Expect(err).NotTo(HaveOccurred())
			Expect(nodeRecovery.Status.CrashLoopBackOffPods).To(BeTrue())
			validateConditions(nodeRecovery, 2, v1alpha1.CleanupOSDRemovalJob, "")
		})

		It("Validates the condition where force deleting the OSD pods through the OSD removal job when there are 3 OSD instances", func() {
			By("Creating the CR")
			template := newTemplate()
			nodeRecovery = getNodeRecoveryWithStatus(v1alpha1.ProcessOCSRemovalTemplate)
			nodeRecovery.Status.CrashLoopBackOffPods = true
			nodeRecovery.Status.CrashedOSDDeploymentIDs = []string{"1"}
			k8sClient = fakeClientBuilder.WithRuntimeObjects(nodeRecovery, template).Build()
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

			By("Validating the arguments in the template")
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "noderec-configmap", Namespace: ODF_NAMESPACE}, cm)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Data[FAILED_OSD_IDS]).To(Equal("1"))
			Expect(cm.Data[FORCE_OSD_REMOVAL]).To(Equal("true"))

			By("Validating the CR status")
			err = k8sClient.Get(ctx, typeNamespacedName, nodeRecovery)
			Expect(err).NotTo(HaveOccurred())
			Expect(nodeRecovery.Status.CrashLoopBackOffPods).To(BeTrue())
			validateConditions(nodeRecovery, 2, v1alpha1.CleanupOSDRemovalJob, "")
		})

		It("Validates the condition of cleaning up the osd removal job status when job times out", func() {
			template := newTemplate()
			nodeRecovery = getNodeRecoveryWithStatus(v1alpha1.CleanupOSDRemovalJob)
			nodeRecovery.Status.Conditions[0].LastTransitionTime = metav1.Time{Time: time.Now().Add(-osdRemovalJobTimeout)}
			nodeRecovery.Status.CrashedOSDDeploymentIDs = []string{"0"}
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
			k8sClient = fakeClientBuilder.WithRuntimeObjects(nodeRecovery, p1, template).Build()
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
			err = k8sClient.Get(ctx, typeNamespacedName, nodeRecovery)
			Expect(err).NotTo(HaveOccurred())
			Expect(nodeRecovery.Status.ForcedOSDRemoval).To(BeTrue())
			validateConditions(nodeRecovery, 2, v1alpha1.RetryForceCleanupOSDRemovalJob, "")
		})

		It("Validates the condition of force cleaning up the osd removal job status when it times out ", func() {
			template := newTemplate()
			nodeRecovery = getNodeRecoveryWithStatus(v1alpha1.RetryForceCleanupOSDRemovalJob)
			nodeRecovery.Status.Conditions[0].LastTransitionTime = metav1.Time{Time: time.Now().Add(-osdRemovalJobTimeout)}
			nodeRecovery.Status.CrashedOSDDeploymentIDs = []string{"0"}
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
			k8sClient = fakeClientBuilder.WithRuntimeObjects(nodeRecovery, p1, template).Build()
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
			Expect(resp.Requeue).To(BeFalse())

			By("Validating the CR status")
			err = k8sClient.Get(ctx, typeNamespacedName, nodeRecovery)
			Expect(err).NotTo(HaveOccurred())
			Expect(nodeRecovery.Status.Phase).To(Equal(v1alpha1.FailedPhase))
			validateConditions(nodeRecovery, 1, v1alpha1.RetryForceCleanupOSDRemovalJob, "")
		})

		It("Validates the condition of force cleaning up the osd removal job status when it completes with success", func() {
			nodeRecovery = getNodeRecoveryWithStatus(v1alpha1.RetryForceCleanupOSDRemovalJob)
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
					Phase: corev1.PodSucceeded,
				},
			}
			k8sClient = fakeClientBuilder.WithRuntimeObjects(nodeRecovery, p1).Build()
			Expect(k8sClient).NotTo(BeNil())
			controllerReconciler = &NodeRecoveryReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				Config:    cfg,
				Recorder:  record.NewFakeRecorder(2),
				CmdRunner: newFakeRemoteExecutor("", "", nil),
				LogClient: newFakeLogRetriever("completed removal"),
			}

			By("Reconciling the created resource")
			resp, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Validating the response")
			Expect(resp.Requeue).To(BeTrue())

			By("Validating the CR status")
			err = k8sClient.Get(ctx, typeNamespacedName, nodeRecovery)
			Expect(err).NotTo(HaveOccurred())
			validateConditions(nodeRecovery, 2, v1alpha1.WaitForPersistenVolumeBound, "")
		})

		It("Validates the condition of cleaning up the osd removal job status when not yet succeeded", func() {
			nodeRecovery = getNodeRecoveryWithStatus(v1alpha1.CleanupOSDRemovalJob)

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
			k8sClient = fakeClientBuilder.WithRuntimeObjects(nodeRecovery, p1).Build()
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
			err = k8sClient.Get(ctx, typeNamespacedName, nodeRecovery)
			Expect(err).NotTo(HaveOccurred())
			validateConditions(nodeRecovery, 1, v1alpha1.CleanupOSDRemovalJob, "")
		})

		It("Validates the condition of cleaning up the osd removal job status when succeeded but pod logs show failure", func() {
			nodeRecovery = getNodeRecoveryWithStatus(v1alpha1.CleanupOSDRemovalJob)
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

			k8sClient = fakeClientBuilder.WithRuntimeObjects(nodeRecovery, p1, job).Build()
			Expect(k8sClient).NotTo(BeNil())
			controllerReconciler = &NodeRecoveryReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				Config:    cfg,
				Recorder:  record.NewFakeRecorder(2),
				CmdRunner: newFakeRemoteExecutor("", "", nil),
				LogClient: newFakeLogRetriever("failed to complete"),
			}

			By("Reconciling the created resource")
			resp, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).To(HaveOccurred())

			By("Validating the response")
			Expect(resp.Requeue).To(BeFalse())

			By("Validating the CR status")
			err = k8sClient.Get(ctx, typeNamespacedName, nodeRecovery)
			Expect(err).NotTo(HaveOccurred())
			validateConditions(nodeRecovery, 1, v1alpha1.CleanupOSDRemovalJob, "")
			By("Validating the osd-removal-job pod has been deleted")
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: job.Namespace, Name: job.Name}, job)
			Expect(kerrors.IsNotFound(err)).NotTo(BeTrue())
		})

		It("Validates the condition of cleaning up the osd removal job status when succeeded and pod logs show success and the job pod is not found", func() {
			nodeRecovery = getNodeRecoveryWithStatus(v1alpha1.CleanupOSDRemovalJob)
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

			k8sClient = fakeClientBuilder.WithRuntimeObjects(nodeRecovery, p1, job).Build()
			Expect(k8sClient).NotTo(BeNil())
			controllerReconciler = &NodeRecoveryReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				Config:    cfg,
				Recorder:  record.NewFakeRecorder(2),
				CmdRunner: newFakeRemoteExecutor("", "", nil),
				LogClient: newFakeLogRetriever("completed removal"),
			}

			By("Reconciling the created resource")
			resp, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Validating the response")
			Expect(resp.Requeue).To(BeTrue())

			By("Validating the CR status")
			err = k8sClient.Get(ctx, typeNamespacedName, nodeRecovery)
			Expect(err).NotTo(HaveOccurred())
			validateConditions(nodeRecovery, 2, v1alpha1.WaitForPersistenVolumeBound, "")
			By("Validating the osd-removal-job pod has been deleted")
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: job.Namespace, Name: job.Name}, job)
			Expect(kerrors.IsNotFound(err)).To(BeTrue())
		})

		It("Validates the condition of deleting the failed PV", func() {
			nodeRecovery = getNodeRecoveryWithStatus(v1alpha1.WaitForPersistenVolumeBound)
			nodeRecovery.Status.NodeDevice = []v1alpha1.NodePV{{PersistentVolumeName: "pvName", NodeName: "foo"}}
			By("Creating the PV ")
			pv := &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pvName",
					Annotations: map[string]string{
						localv1.PVDeviceNameLabel: "vdb",
					},
					Labels: map[string]string{
						"kubernetes.io/hostname": "foo",
					},
				},
				Status: corev1.PersistentVolumeStatus{
					Phase: corev1.VolumeReleased,
				},
			}

			k8sClient = fakeClientBuilder.WithRuntimeObjects(nodeRecovery, pv).Build()
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
			err = k8sClient.Get(ctx, typeNamespacedName, nodeRecovery)
			Expect(err).NotTo(HaveOccurred())
			validateConditions(nodeRecovery, 1, v1alpha1.WaitForPersistenVolumeBound, "")
		})

		It("Validates the condition of PV bound after being released", func() {
			nodeRecovery = getNodeRecoveryWithStatus(v1alpha1.WaitForPersistenVolumeBound)
			nodeRecovery.Status.NodeDevice = []v1alpha1.NodePV{{PersistentVolumeName: "pvName", NodeName: "foo"}}
			By("Creating the PV ")
			pv := &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pvName",
					Annotations: map[string]string{
						localv1.PVDeviceNameLabel: "vdb",
					},
					Labels: map[string]string{
						"kubernetes.io/hostname": "foo",
					},
				},
				Status: corev1.PersistentVolumeStatus{
					Phase: corev1.VolumeBound,
				},
			}

			k8sClient = fakeClientBuilder.WithRuntimeObjects(nodeRecovery, pv).Build()
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
			err = k8sClient.Get(ctx, typeNamespacedName, nodeRecovery)
			Expect(err).NotTo(HaveOccurred())
			validateConditions(nodeRecovery, 2, v1alpha1.RestartStorageOperator, "")
		})

		It("Validates the condition of restarting the storage operator when pending pods were found", func() {
			nodeRecovery = getNodeRecoveryWithStatus(v1alpha1.RestartStorageOperator)
			nodeRecovery.Status.PendingPods = true
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
			k8sClient = fakeClientBuilder.WithRuntimeObjects(nodeRecovery, p).Build()
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
			Expect(resp.RequeueAfter).To(Equal(30 * time.Second))

			By("Validating the CR status")
			err = k8sClient.Get(ctx, typeNamespacedName, nodeRecovery)
			Expect(err).NotTo(HaveOccurred())
			validateConditions(nodeRecovery, 2, v1alpha1.DeleteFailedPodsNodeAffinity, "")
		})

		It("Validates the condition of restarting the storage operator when pods with crashloopbackoff status were found", func() {
			nodeRecovery = getNodeRecoveryWithStatus(v1alpha1.RestartStorageOperator)
			nodeRecovery.Status.CrashLoopBackOffPods = true

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

			k8sClient = fakeClientBuilder.WithRuntimeObjects(nodeRecovery, p).Build()
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
			Expect(resp.RequeueAfter).To(Equal(30 * time.Second))

			By("Validating the CR status")
			err = k8sClient.Get(ctx, typeNamespacedName, nodeRecovery)
			Expect(err).NotTo(HaveOccurred())
			validateConditions(nodeRecovery, 2, v1alpha1.DeleteFailedPodsNodeAffinity, "")
		})

		It("Validates the condition of deleting the failed pods due to node affinity", func() {
			nodeRecovery = getNodeRecoveryWithStatus(v1alpha1.DeleteFailedPodsNodeAffinity)

			init := newOCSInit(enabledCephTools)

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

			k8sClient = fakeClientBuilder.WithRuntimeObjects(init, nodeRecovery, tools, p).Build()
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
			err = k8sClient.Get(ctx, typeNamespacedName, nodeRecovery)
			Expect(err).NotTo(HaveOccurred())
			validateConditions(nodeRecovery, 2, v1alpha1.ArchiveCephDaemonCrashMessages, "")
		})

		It("Validates the condition of archieving the ceph daemon crash messages", func() {
			nodeRecovery = getNodeRecoveryWithStatus(v1alpha1.ArchiveCephDaemonCrashMessages)

			init := newOCSInit(enabledCephTools)

			By("Creating a the ceph tools pod")

			tools := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "storage-operator",
					Namespace: "openshift-storage",
					Labels: map[string]string{
						"app": "rook-ceph-tools",
					},
				},
			}

			k8sClient = fakeClientBuilder.WithRuntimeObjects(init, nodeRecovery, tools).Build()
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
			err = k8sClient.Get(ctx, typeNamespacedName, nodeRecovery)
			Expect(err).NotTo(HaveOccurred())
			validateConditions(nodeRecovery, 2, v1alpha1.StorageClusterFitnessCheck, "")
		})

		It("Validates the condition of storage cluster fitness check", func() {
			nodeRecovery = getNodeRecoveryWithStatus(v1alpha1.StorageClusterFitnessCheck)
			init := newOCSInit(enabledCephTools)
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
			k8sClient = fakeClientBuilder.WithRuntimeObjects(nodeRecovery, init, p).Build()
			Expect(k8sClient).NotTo(BeNil())
			controllerReconciler = &NodeRecoveryReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				Config:    cfg,
				Recorder:  record.NewFakeRecorder(2),
				CmdRunner: newFakeRemoteExecutor(`{"health":{"status":"HEALTH_OK"}}`, "", nil),
			}

			By("Reconciling the CR")
			resp, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Validating the response")
			Expect(resp.Requeue).To(BeTrue())

			By("Validating the CR status")
			err = k8sClient.Get(ctx, typeNamespacedName, nodeRecovery)
			Expect(err).NotTo(HaveOccurred())
			validateConditions(nodeRecovery, 2, v1alpha1.DisableCephTools, "")
		})

		It("Validates the condition of disabling the ceph tools pod when it was initially found disabled", func() {
			nodeRecovery = getNodeRecoveryWithStatus(v1alpha1.DisableCephTools)
			init := newOCSInit(enabledCephTools)
			k8sClient = fakeClientBuilder.WithRuntimeObjects(nodeRecovery, init).Build()
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
			err = k8sClient.Get(ctx, typeNamespacedName, nodeRecovery)
			Expect(err).NotTo(HaveOccurred())
			Expect(nodeRecovery.Status.Phase).To(Equal(v1alpha1.CompletedPhase))
			Expect(nodeRecovery.Status.CompletionTime.Time.IsZero()).To(BeFalse())
			Expect(nodeRecovery.Status.Conditions).To(HaveLen(1))
		})

		It("Validates the condition of disabling the ceph tools pod when it was initially found enabled", func() {
			nodeRecovery = getNodeRecoveryWithStatus(v1alpha1.DisableCephTools)
			init := newOCSInit(enabledCephTools)
			nodeRecovery.Status.KeepCephToolsPod = true
			k8sClient = fakeClientBuilder.WithRuntimeObjects(nodeRecovery, init).Build()
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
			err = k8sClient.Get(ctx, typeNamespacedName, nodeRecovery)
			Expect(err).NotTo(HaveOccurred())
			Expect(nodeRecovery.Status.Phase).To(Equal(v1alpha1.CompletedPhase))
			Expect(nodeRecovery.Status.CompletionTime.Time.IsZero()).To(BeFalse())
			Expect(nodeRecovery.Status.Conditions).To(HaveLen(1))
		})

	})
})

type cephToolsState bool

const (
	enabledCephTools  cephToolsState = true
	disabledCephTools cephToolsState = false
)

type fakeLogRetriever struct {
	stdout string
}

func (f *fakeLogRetriever) GetLogs(podName string) (string, error) {
	return f.stdout, nil
}

func newFakeLogRetriever(stdout string) podLogRetriever {
	return &fakeLogRetriever{stdout: stdout}
}

type fakeRemoteExecutor struct {
	stdout, stderr string
	err            error
}

func newFakeRemoteExecutor(stdout, stderr string, err error) *fakeRemoteExecutor {
	return &fakeRemoteExecutor{stdout: stdout, stderr: stderr, err: err}
}

func (f *fakeRemoteExecutor) Run(pod *corev1.Pod, cmd []string) ([]byte, []byte, error) {
	return []byte(f.stdout), []byte(f.stderr), f.err
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

func getNodeRecoveryWithStatus(status v1alpha1.RecoveryConditionType) *v1alpha1.NodeRecovery {
	return &v1alpha1.NodeRecovery{
		ObjectMeta: metav1.ObjectMeta{
			Name: resourceName,
		},
		Status: v1alpha1.NodeRecoveryStatus{
			Phase:     v1alpha1.RunningPhase,
			StartTime: &metav1.Time{Time: time.Now()},
			Conditions: []v1alpha1.RecoveryCondition{
				{Type: status,
					LastProbeTime:      metav1.NewTime(time.Now()),
					LastTransitionTime: metav1.NewTime(time.Now())},
			},
		},
	}
}

func newOCPVersion(version string) *configv1.ClusterVersion {
	return &configv1.ClusterVersion{
		ObjectMeta: metav1.ObjectMeta{Name: "version"},
		Status: configv1.ClusterVersionStatus{
			History: []configv1.UpdateHistory{
				{State: configv1.CompletedUpdate,
					Version: version},
			},
		},
	}
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
    "name": "`+OCS_OSD_REMOVAL_JOB+`",
    "namespace": "`+ODF_NAMESPACE+`"
  },
  "objects": [
    {
      "kind": "ConfigMap",
      "apiVersion": "v1",
      "metadata": {
        "name": "noderec-configmap",
        "namespace": "openshift-storage"
      },
      "data": {
        "FAILED_OSD_IDS": "${FAILED_OSD_IDS}",
        "FORCE_OSD_REMOVAL": "${FORCE_OSD_REMOVAL}"
      }
    }
  ],
  "parameters": [
    {
      "name": "FAILED_OSD_IDS",
      "required": true
    },
    {
      "name": "FORCE_OSD_REMOVAL",
      "required": true
    }
  ]
}`), &template)
	Expect(err).NotTo(HaveOccurred())
	return &template
}

func generateOSDPodObject(id string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pv-pod-" + id,
			Namespace: "openshift-storage",
			Labels: map[string]string{
				osd.OSDOverPVCLabelKey: "pvcName",
				"app":                  "rook-ceph-osd",
				osd.OsdIdLabelKey:      id,
			},
		},
		Status: corev1.PodStatus{
			InitContainerStatuses: []corev1.ContainerStatus{
				{State: corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{}}}},
		},
	}
}

func validateConditions(nodeRecovery *v1alpha1.NodeRecovery, numConditions int, lastCondition v1alpha1.RecoveryConditionType, reason v1alpha1.RecoveryConditionReason) {
	Expect(nodeRecovery.Status.Conditions).To(HaveLen(numConditions))
	Expect(nodeRecovery.Status.Conditions[len(nodeRecovery.Status.Conditions)-1].Type).To(Equal(lastCondition))
	Expect(nodeRecovery.Status.Conditions[len(nodeRecovery.Status.Conditions)-1].Reason).To(Equal(reason))
}

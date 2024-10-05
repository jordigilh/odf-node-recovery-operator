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

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NodeRecoverySpec defines the desired state of NodeRecovery
type NodeRecoverySpec struct {
	//empty by design.
}

type RecoveryConditionType string
type RecoveryConditionReason string

var (
	EnableCephToolsPod           RecoveryConditionType = "EnableCephToolsPod"
	WaitForCephToolsPodRunning   RecoveryConditionType = "WaitForCephToolsPodRunning"
	WaitForOSDPodsStabilize      RecoveryConditionType = "WaitForOSDPodsStabilize"
	LabelNodesWithPendingPods    RecoveryConditionType = "LabelNodesWithPendingPods"
	ManageCrashLoopBackOffPods   RecoveryConditionType = "ManageCrashLoopBackOffPods"
	CleanupOSDRemovalJob         RecoveryConditionType = "CleanupOSDRemovalJob"
	RestartStorageOperator       RecoveryConditionType = "RestartStorageOperator"
	DeleteFailedPodsNodeAffinity RecoveryConditionType = "DeleteFailedPodsNodeAffinity"
	StorageClusterFitnessCheck   RecoveryConditionType = "StorageClusterFitnessCheck"
	DisableCephTools             RecoveryConditionType = "DisableCephTools"

	RunningPhase   RecoveryPhase = "Running"
	FailedPhase    RecoveryPhase = "Failed"
	CompletedPhase RecoveryPhase = "Completed"

	FailedCheckCephToolsPod              RecoveryConditionReason = "FailedCheckCephToolsPod"
	FailedRetrieveCephToolPod            RecoveryConditionReason = "FailedRetrieveCephToolPod"
	FailedEnableCephToolsPod             RecoveryConditionReason = "FailedEnableCephToolsPod"
	FailedDisableCephToolsPod            RecoveryConditionReason = "FailedDisableCephToolsPod"
	PodNotInRunningPhase                 RecoveryConditionReason = "PodNotInRunningPhase"
	FailedRetrievePodsPhase              RecoveryConditionReason = "FailedRetrievePodsPhase"
	WaitingForPodsToInitialize           RecoveryConditionReason = "WaitingForPodsToInitialize"
	FailedRetrievePendingPods            RecoveryConditionReason = "FailedRetrievePendingPods"
	FailedLabelNodes                     RecoveryConditionReason = "FailedLabelNodes"
	FailedRetrieveCrashLoopBackOffPods   RecoveryConditionReason = "FailedRetrieveCrashLoopBackOffPods"
	FailedHandleCrashLoopBackOffPods     RecoveryConditionReason = "FailedHandleCrashLoopBackOffPods"
	FailedRestartODFOperator             RecoveryConditionReason = "FailedRestartODFOperator"
	FailedDeleteFailedPodsNodeAffinity   RecoveryConditionReason = "FailedDeleteFailedPodsNodeAffinity"
	FailedArchiveCephDaemonCrashMessages RecoveryConditionReason = "FailedArchiveCephDaemonCrashMessages"
	FailedRetrieveCephHealthStatus       RecoveryConditionReason = "FailedRetrieveCephHealthStatus"
)

// PodCondition contains details for the current condition of this pod.
type RecoveryCondition struct {
	// Type is the type of the condition.
	Type RecoveryConditionType `json:"type" protobuf:"bytes,1,opt,name=type,casttype=conditionType"`
	// Status is the status of the condition.
	// Can be True, False, Unknown.
	Status v1.ConditionStatus `json:"status" protobuf:"bytes,2,opt,name=status,casttype=ConditionStatus"`
	// Last time we probed the condition.
	LastProbeTime metav1.Time `json:"lastProbeTime" protobuf:"bytes,3,opt,name=lastProbeTime"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime" protobuf:"bytes,4,opt,name=lastTransitionTime"`
	// Unique, one-word, CamelCase reason for the condition's last transition.
	// +optional
	Reason RecoveryConditionReason `json:"reason,omitempty" protobuf:"bytes,5,opt,name=reason"`
	// Human-readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty" protobuf:"bytes,6,opt,name=message"`
}

type RecoveryPhase string

// NodeRecoveryStatus defines the observed state of NodeRecovery
type NodeRecoveryStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Phase RecoveryPhase `json:"phase,omitempty" protobuf:"bytes,1,opt,name=phase,casttype=RecoveryPhase"`
	// Current conditions state of CR.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []RecoveryCondition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,2,rep,name=conditions"`

	StartTime *metav1.Time `json:"startTime,omitempty" protobuf:"bytes,2,opt,name=startTime"`

	// Represents time when the job was completed. It is not guaranteed to
	// be set in happens-before order across separate operations.
	// It is represented in RFC3339 form and is in UTC.
	// The completion time is set when the reconciliation finishes successfully, and only then.
	// The value cannot be updated or removed. The value indicates the same or
	// later point in time as the startTime field.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty" protobuf:"bytes,3,opt,name=completionTime"`

	//PendingPods captures whether there were OSD pods in pending phase when reconciling the CR. This value is used by the reconciler along with `CrashLoopBackOffPods` to determine
	// if the reconciliation requires a restart of the ODF operator.
	PendingPods bool `json:"pendingPods,omitempty"`
	//CrashLoopBackOffPods captures whether there were OSD pods in CrashLoopBackOff when reconciling the CR. This value is used by the reconciler along with the `PendingPods` to determine
	// if the reconciliation requires a restart the ODF operator.
	CrashLoopBackOffPods bool `json:"crashLoopBackOffPods,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// NodeRecovery is the Schema for the noderecoveries API
type NodeRecovery struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeRecoverySpec   `json:"spec,omitempty"`
	Status NodeRecoveryStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// NodeRecoveryList contains a list of NodeRecovery
type NodeRecoveryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeRecovery `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeRecovery{}, &NodeRecoveryList{})
}

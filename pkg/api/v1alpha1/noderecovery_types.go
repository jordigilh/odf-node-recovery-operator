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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type RecoveryConditionType string
type RecoveryConditionReason string

var (
	EnableCephToolsPod             RecoveryConditionType = "EnableCephToolsPod"
	WaitForCephToolsPodRunning     RecoveryConditionType = "WaitForCephToolsPodRunning"
	WaitForOSDPodsStabilize        RecoveryConditionType = "WaitForOSDPodsStabilize"
	LabelNodesWithPendingPods      RecoveryConditionType = "LabelNodesWithPendingPods"
	ManageCrashLoopBackOffPods     RecoveryConditionType = "ManageCrashLoopBackOffPods"
	ForceDeleteRookCephOSDPods     RecoveryConditionType = "ForceDeleteRookCephOSDPods"
	ProcessOCSRemovalTemplate      RecoveryConditionType = "ProcessOCSRemovalTemplate"
	CleanupOSDRemovalJob           RecoveryConditionType = "CleanupOSDRemovalJob"
	RetryForceCleanupOSDRemovalJob RecoveryConditionType = "RetryForceCleanupOSDRemovalJob"
	// DeletePersistentVolume         RecoveryConditionType = "DeletePersistentVolume"
	WaitForPersistenVolumeBound    RecoveryConditionType = "WaitForPersistenVolumeBound"
	RestartStorageOperator         RecoveryConditionType = "RestartStorageOperator"
	DeleteFailedPodsNodeAffinity   RecoveryConditionType = "DeleteFailedPodsNodeAffinity"
	ArchiveCephDaemonCrashMessages RecoveryConditionType = "ArchiveCephDaemonCrashMessages"
	StorageClusterFitnessCheck     RecoveryConditionType = "StorageClusterFitnessCheck"
	DisableCephTools               RecoveryConditionType = "DisableCephTools"

	RunningPhase   RecoveryPhase = "Running"
	FailedPhase    RecoveryPhase = "Failed"
	CompletedPhase RecoveryPhase = "Completed"

	StatusTrue  StatusType = "True"
	StatusFalse StatusType = "False"

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

	// Status is the status of the condition. Can be True, False, Unknown
	// +kubebuilder:validation:Enum={"True","False"}
	Status StatusType `json:"status,omitempty"`
}

type StatusType string

type RecoveryPhase string

// NodeRecoveryStatus defines the observed state of NodeRecovery
type NodeRecoveryStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// +kubebuilder:validation:Enum={"Running","Completed", "Failed"}
	Phase RecoveryPhase `json:"phase,omitempty" protobuf:"bytes,1,opt,name=phase,casttype=RecoveryPhase"`
	// Current conditions state of CR.
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:Optional
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

	//NodeDevice contains a list of node name and device name pair used by the reconciliation to track which nodes and devices have failed based on the OSD pods that are in CrashLoopbackOff
	NodeDevice []NodePV `json:"nodeDevice,omitempty"`

	// CrashedOSDDeploymentIDs contains a list of the OSD IDs that match the ceph osd pods that are in CrashLoopbackOff status. This value is used during the reconciliation loop to cleanup the
	// pods that are not being removed when the deployment is scaled down to 0
	CrashedOSDDeploymentIDs []string `json:"osdIDs,omitempty"`

	// ForcedOSDRemoval indicates if the reconciliation of the CR required to trigger the OSD Removal job with the ForcedOSDRemoval flag as true or false. If true it means the initial attempt to run the job timed out after 10 minutes
	// and the reconciliation loop triggered a second job with the flag set to true to ensure success.
	ForcedOSDRemoval bool `json:"forcedOSDRemoval,omitempty"`

	// KeepCephToolsPod indicates if the reconciliation shall delete the CephTools pod after completing the reconciliation flow. By default it will remove the pod unless it detects
	// the pod exists prior to starting the reconciilation
	KeepCephToolsPod bool `json:"keepCephToolsPod,omitempty"`
}

// NodeRecovery is the Schema for the noderecoveries API
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=noderec
// +kubebuilder:printcolumn:name="Created At",type=string,JSONPath=.status.startTime
// +kubebuilder:printcolumn:name="Completed At",type=string,JSONPath=.status.completionTime
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=.status.phase,description="Status"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=.status.conditions[?(@.status=="True")].type
type NodeRecovery struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Status NodeRecoveryStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// NodeRecoveryList contains a list of NodeRecovery
type NodeRecoveryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeRecovery `json:"items"`
}

type NodePV struct {
	NodeName             string `json:"nodeName"`
	PersistentVolumeName string `json:"pvName"`
}

func init() {
	SchemeBuilder.Register(&NodeRecovery{}, &NodeRecoveryList{})
}

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
// Code generated by applyconfiguration-gen. DO NOT EDIT.

package v1alpha1

import (
	v1alpha1 "github.com/jordigilh/odf-node-recovery-operator/pkg/api/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeRecoveryStatusApplyConfiguration represents a declarative configuration of the NodeRecoveryStatus type for use
// with apply.
type NodeRecoveryStatusApplyConfiguration struct {
	Phase                *v1alpha1.RecoveryPhase               `json:"phase,omitempty"`
	Conditions           []RecoveryConditionApplyConfiguration `json:"conditions,omitempty"`
	StartTime            *v1.Time                              `json:"startTime,omitempty"`
	CompletionTime       *v1.Time                              `json:"completionTime,omitempty"`
	PendingPods          *bool                                 `json:"pendingPods,omitempty"`
	CrashLoopBackOffPods *bool                                 `json:"crashLoopBackOffPods,omitempty"`
}

// NodeRecoveryStatusApplyConfiguration constructs a declarative configuration of the NodeRecoveryStatus type for use with
// apply.
func NodeRecoveryStatus() *NodeRecoveryStatusApplyConfiguration {
	return &NodeRecoveryStatusApplyConfiguration{}
}

// WithPhase sets the Phase field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Phase field is set to the value of the last call.
func (b *NodeRecoveryStatusApplyConfiguration) WithPhase(value v1alpha1.RecoveryPhase) *NodeRecoveryStatusApplyConfiguration {
	b.Phase = &value
	return b
}

// WithConditions adds the given value to the Conditions field in the declarative configuration
// and returns the receiver, so that objects can be build by chaining "With" function invocations.
// If called multiple times, values provided by each call will be appended to the Conditions field.
func (b *NodeRecoveryStatusApplyConfiguration) WithConditions(values ...*RecoveryConditionApplyConfiguration) *NodeRecoveryStatusApplyConfiguration {
	for i := range values {
		if values[i] == nil {
			panic("nil value passed to WithConditions")
		}
		b.Conditions = append(b.Conditions, *values[i])
	}
	return b
}

// WithStartTime sets the StartTime field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the StartTime field is set to the value of the last call.
func (b *NodeRecoveryStatusApplyConfiguration) WithStartTime(value v1.Time) *NodeRecoveryStatusApplyConfiguration {
	b.StartTime = &value
	return b
}

// WithCompletionTime sets the CompletionTime field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the CompletionTime field is set to the value of the last call.
func (b *NodeRecoveryStatusApplyConfiguration) WithCompletionTime(value v1.Time) *NodeRecoveryStatusApplyConfiguration {
	b.CompletionTime = &value
	return b
}

// WithPendingPods sets the PendingPods field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the PendingPods field is set to the value of the last call.
func (b *NodeRecoveryStatusApplyConfiguration) WithPendingPods(value bool) *NodeRecoveryStatusApplyConfiguration {
	b.PendingPods = &value
	return b
}

// WithCrashLoopBackOffPods sets the CrashLoopBackOffPods field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the CrashLoopBackOffPods field is set to the value of the last call.
func (b *NodeRecoveryStatusApplyConfiguration) WithCrashLoopBackOffPods(value bool) *NodeRecoveryStatusApplyConfiguration {
	b.CrashLoopBackOffPods = &value
	return b
}

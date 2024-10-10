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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RecoveryConditionApplyConfiguration represents a declarative configuration of the RecoveryCondition type for use
// with apply.
type RecoveryConditionApplyConfiguration struct {
	Type               *v1alpha1.RecoveryConditionType   `json:"type,omitempty"`
	Status             *v1.ConditionStatus               `json:"status,omitempty"`
	LastProbeTime      *metav1.Time                      `json:"lastProbeTime,omitempty"`
	LastTransitionTime *metav1.Time                      `json:"lastTransitionTime,omitempty"`
	Reason             *v1alpha1.RecoveryConditionReason `json:"reason,omitempty"`
	Message            *string                           `json:"message,omitempty"`
}

// RecoveryConditionApplyConfiguration constructs a declarative configuration of the RecoveryCondition type for use with
// apply.
func RecoveryCondition() *RecoveryConditionApplyConfiguration {
	return &RecoveryConditionApplyConfiguration{}
}

// WithType sets the Type field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Type field is set to the value of the last call.
func (b *RecoveryConditionApplyConfiguration) WithType(value v1alpha1.RecoveryConditionType) *RecoveryConditionApplyConfiguration {
	b.Type = &value
	return b
}

// WithStatus sets the Status field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Status field is set to the value of the last call.
func (b *RecoveryConditionApplyConfiguration) WithStatus(value v1.ConditionStatus) *RecoveryConditionApplyConfiguration {
	b.Status = &value
	return b
}

// WithLastProbeTime sets the LastProbeTime field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the LastProbeTime field is set to the value of the last call.
func (b *RecoveryConditionApplyConfiguration) WithLastProbeTime(value metav1.Time) *RecoveryConditionApplyConfiguration {
	b.LastProbeTime = &value
	return b
}

// WithLastTransitionTime sets the LastTransitionTime field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the LastTransitionTime field is set to the value of the last call.
func (b *RecoveryConditionApplyConfiguration) WithLastTransitionTime(value metav1.Time) *RecoveryConditionApplyConfiguration {
	b.LastTransitionTime = &value
	return b
}

// WithReason sets the Reason field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Reason field is set to the value of the last call.
func (b *RecoveryConditionApplyConfiguration) WithReason(value v1alpha1.RecoveryConditionReason) *RecoveryConditionApplyConfiguration {
	b.Reason = &value
	return b
}

// WithMessage sets the Message field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Message field is set to the value of the last call.
func (b *RecoveryConditionApplyConfiguration) WithMessage(value string) *RecoveryConditionApplyConfiguration {
	b.Message = &value
	return b
}
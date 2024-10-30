/*
Copyright The Kubernetes Authors.

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

package v1

import (
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// CustomResourceSubresourcesApplyConfiguration represents a declarative configuration of the CustomResourceSubresources type for use
// with apply.
type CustomResourceSubresourcesApplyConfiguration struct {
	Status *v1.CustomResourceSubresourceStatus               `json:"status,omitempty"`
	Scale  *CustomResourceSubresourceScaleApplyConfiguration `json:"scale,omitempty"`
}

// CustomResourceSubresourcesApplyConfiguration constructs a declarative configuration of the CustomResourceSubresources type for use with
// apply.
func CustomResourceSubresources() *CustomResourceSubresourcesApplyConfiguration {
	return &CustomResourceSubresourcesApplyConfiguration{}
}

// WithStatus sets the Status field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Status field is set to the value of the last call.
func (b *CustomResourceSubresourcesApplyConfiguration) WithStatus(value v1.CustomResourceSubresourceStatus) *CustomResourceSubresourcesApplyConfiguration {
	b.Status = &value
	return b
}

// WithScale sets the Scale field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Scale field is set to the value of the last call.
func (b *CustomResourceSubresourcesApplyConfiguration) WithScale(value *CustomResourceSubresourceScaleApplyConfiguration) *CustomResourceSubresourcesApplyConfiguration {
	b.Scale = value
	return b
}
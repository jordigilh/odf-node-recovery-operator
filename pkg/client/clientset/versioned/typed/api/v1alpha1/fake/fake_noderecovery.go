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
// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	"context"
	json "encoding/json"
	"fmt"

	v1alpha1 "github.com/jordigilh/odf-node-recovery-operator/pkg/api/v1alpha1"
	apiv1alpha1 "github.com/jordigilh/odf-node-recovery-operator/pkg/client/applyconfiguration/api/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeNodeRecoveries implements NodeRecoveryInterface
type FakeNodeRecoveries struct {
	Fake *FakeApiV1alpha1
}

var noderecoveriesResource = v1alpha1.SchemeGroupVersion.WithResource("noderecoveries")

var noderecoveriesKind = v1alpha1.SchemeGroupVersion.WithKind("NodeRecovery")

// Get takes name of the nodeRecovery, and returns the corresponding nodeRecovery object, and an error if there is any.
func (c *FakeNodeRecoveries) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.NodeRecovery, err error) {
	emptyResult := &v1alpha1.NodeRecovery{}
	obj, err := c.Fake.
		Invokes(testing.NewRootGetActionWithOptions(noderecoveriesResource, name, options), emptyResult)
	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v1alpha1.NodeRecovery), err
}

// List takes label and field selectors, and returns the list of NodeRecoveries that match those selectors.
func (c *FakeNodeRecoveries) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.NodeRecoveryList, err error) {
	emptyResult := &v1alpha1.NodeRecoveryList{}
	obj, err := c.Fake.
		Invokes(testing.NewRootListActionWithOptions(noderecoveriesResource, noderecoveriesKind, opts), emptyResult)
	if obj == nil {
		return emptyResult, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.NodeRecoveryList{ListMeta: obj.(*v1alpha1.NodeRecoveryList).ListMeta}
	for _, item := range obj.(*v1alpha1.NodeRecoveryList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested nodeRecoveries.
func (c *FakeNodeRecoveries) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewRootWatchActionWithOptions(noderecoveriesResource, opts))
}

// Create takes the representation of a nodeRecovery and creates it.  Returns the server's representation of the nodeRecovery, and an error, if there is any.
func (c *FakeNodeRecoveries) Create(ctx context.Context, nodeRecovery *v1alpha1.NodeRecovery, opts v1.CreateOptions) (result *v1alpha1.NodeRecovery, err error) {
	emptyResult := &v1alpha1.NodeRecovery{}
	obj, err := c.Fake.
		Invokes(testing.NewRootCreateActionWithOptions(noderecoveriesResource, nodeRecovery, opts), emptyResult)
	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v1alpha1.NodeRecovery), err
}

// Update takes the representation of a nodeRecovery and updates it. Returns the server's representation of the nodeRecovery, and an error, if there is any.
func (c *FakeNodeRecoveries) Update(ctx context.Context, nodeRecovery *v1alpha1.NodeRecovery, opts v1.UpdateOptions) (result *v1alpha1.NodeRecovery, err error) {
	emptyResult := &v1alpha1.NodeRecovery{}
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateActionWithOptions(noderecoveriesResource, nodeRecovery, opts), emptyResult)
	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v1alpha1.NodeRecovery), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeNodeRecoveries) UpdateStatus(ctx context.Context, nodeRecovery *v1alpha1.NodeRecovery, opts v1.UpdateOptions) (result *v1alpha1.NodeRecovery, err error) {
	emptyResult := &v1alpha1.NodeRecovery{}
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateSubresourceActionWithOptions(noderecoveriesResource, "status", nodeRecovery, opts), emptyResult)
	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v1alpha1.NodeRecovery), err
}

// Delete takes name of the nodeRecovery and deletes it. Returns an error if one occurs.
func (c *FakeNodeRecoveries) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewRootDeleteActionWithOptions(noderecoveriesResource, name, opts), &v1alpha1.NodeRecovery{})
	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeNodeRecoveries) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewRootDeleteCollectionActionWithOptions(noderecoveriesResource, opts, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha1.NodeRecoveryList{})
	return err
}

// Patch applies the patch and returns the patched nodeRecovery.
func (c *FakeNodeRecoveries) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.NodeRecovery, err error) {
	emptyResult := &v1alpha1.NodeRecovery{}
	obj, err := c.Fake.
		Invokes(testing.NewRootPatchSubresourceActionWithOptions(noderecoveriesResource, name, pt, data, opts, subresources...), emptyResult)
	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v1alpha1.NodeRecovery), err
}

// Apply takes the given apply declarative configuration, applies it and returns the applied nodeRecovery.
func (c *FakeNodeRecoveries) Apply(ctx context.Context, nodeRecovery *apiv1alpha1.NodeRecoveryApplyConfiguration, opts v1.ApplyOptions) (result *v1alpha1.NodeRecovery, err error) {
	if nodeRecovery == nil {
		return nil, fmt.Errorf("nodeRecovery provided to Apply must not be nil")
	}
	data, err := json.Marshal(nodeRecovery)
	if err != nil {
		return nil, err
	}
	name := nodeRecovery.Name
	if name == nil {
		return nil, fmt.Errorf("nodeRecovery.Name must be provided to Apply")
	}
	emptyResult := &v1alpha1.NodeRecovery{}
	obj, err := c.Fake.
		Invokes(testing.NewRootPatchSubresourceActionWithOptions(noderecoveriesResource, *name, types.ApplyPatchType, data, opts.ToPatchOptions()), emptyResult)
	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v1alpha1.NodeRecovery), err
}

// ApplyStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating ApplyStatus().
func (c *FakeNodeRecoveries) ApplyStatus(ctx context.Context, nodeRecovery *apiv1alpha1.NodeRecoveryApplyConfiguration, opts v1.ApplyOptions) (result *v1alpha1.NodeRecovery, err error) {
	if nodeRecovery == nil {
		return nil, fmt.Errorf("nodeRecovery provided to Apply must not be nil")
	}
	data, err := json.Marshal(nodeRecovery)
	if err != nil {
		return nil, err
	}
	name := nodeRecovery.Name
	if name == nil {
		return nil, fmt.Errorf("nodeRecovery.Name must be provided to Apply")
	}
	emptyResult := &v1alpha1.NodeRecovery{}
	obj, err := c.Fake.
		Invokes(testing.NewRootPatchSubresourceActionWithOptions(noderecoveriesResource, *name, types.ApplyPatchType, data, opts.ToPatchOptions(), "status"), emptyResult)
	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v1alpha1.NodeRecovery), err
}

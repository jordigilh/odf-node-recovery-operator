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

package v1alpha1

import (
	"context"

	v1alpha1 "github.com/jordigilh/odf-node-recovery-operator/pkg/api/v1alpha1"
	apiv1alpha1 "github.com/jordigilh/odf-node-recovery-operator/pkg/client/applyconfiguration/api/v1alpha1"
	scheme "github.com/jordigilh/odf-node-recovery-operator/pkg/client/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	gentype "k8s.io/client-go/gentype"
)

// NodeRecoveriesGetter has a method to return a NodeRecoveryInterface.
// A group's client should implement this interface.
type NodeRecoveriesGetter interface {
	NodeRecoveries() NodeRecoveryInterface
}

// NodeRecoveryInterface has methods to work with NodeRecovery resources.
type NodeRecoveryInterface interface {
	Create(ctx context.Context, nodeRecovery *v1alpha1.NodeRecovery, opts v1.CreateOptions) (*v1alpha1.NodeRecovery, error)
	Update(ctx context.Context, nodeRecovery *v1alpha1.NodeRecovery, opts v1.UpdateOptions) (*v1alpha1.NodeRecovery, error)
	// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
	UpdateStatus(ctx context.Context, nodeRecovery *v1alpha1.NodeRecovery, opts v1.UpdateOptions) (*v1alpha1.NodeRecovery, error)
	Delete(ctx context.Context, name string, opts v1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error
	Get(ctx context.Context, name string, opts v1.GetOptions) (*v1alpha1.NodeRecovery, error)
	List(ctx context.Context, opts v1.ListOptions) (*v1alpha1.NodeRecoveryList, error)
	Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.NodeRecovery, err error)
	Apply(ctx context.Context, nodeRecovery *apiv1alpha1.NodeRecoveryApplyConfiguration, opts v1.ApplyOptions) (result *v1alpha1.NodeRecovery, err error)
	// Add a +genclient:noStatus comment above the type to avoid generating ApplyStatus().
	ApplyStatus(ctx context.Context, nodeRecovery *apiv1alpha1.NodeRecoveryApplyConfiguration, opts v1.ApplyOptions) (result *v1alpha1.NodeRecovery, err error)
	NodeRecoveryExpansion
}

// nodeRecoveries implements NodeRecoveryInterface
type nodeRecoveries struct {
	*gentype.ClientWithListAndApply[*v1alpha1.NodeRecovery, *v1alpha1.NodeRecoveryList, *apiv1alpha1.NodeRecoveryApplyConfiguration]
}

// newNodeRecoveries returns a NodeRecoveries
func newNodeRecoveries(c *ApiV1alpha1Client) *nodeRecoveries {
	return &nodeRecoveries{
		gentype.NewClientWithListAndApply[*v1alpha1.NodeRecovery, *v1alpha1.NodeRecoveryList, *apiv1alpha1.NodeRecoveryApplyConfiguration](
			"noderecoveries",
			c.RESTClient(),
			scheme.ParameterCodec,
			"",
			func() *v1alpha1.NodeRecovery { return &v1alpha1.NodeRecovery{} },
			func() *v1alpha1.NodeRecoveryList { return &v1alpha1.NodeRecoveryList{} }),
	}
}

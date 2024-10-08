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
// Code generated by lister-gen. DO NOT EDIT.

package v1alpha1

import (
	v1alpha1 "github.com/jordigilh/odf-node-recovery-operator/pkg/api/v1alpha1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/listers"
	"k8s.io/client-go/tools/cache"
)

// NodeRecoveryLister helps list NodeRecoveries.
// All objects returned here must be treated as read-only.
type NodeRecoveryLister interface {
	// List lists all NodeRecoveries in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.NodeRecovery, err error)
	// Get retrieves the NodeRecovery from the index for a given name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1alpha1.NodeRecovery, error)
	NodeRecoveryListerExpansion
}

// nodeRecoveryLister implements the NodeRecoveryLister interface.
type nodeRecoveryLister struct {
	listers.ResourceIndexer[*v1alpha1.NodeRecovery]
}

// NewNodeRecoveryLister returns a new NodeRecoveryLister.
func NewNodeRecoveryLister(indexer cache.Indexer) NodeRecoveryLister {
	return &nodeRecoveryLister{listers.New[*v1alpha1.NodeRecovery](indexer, v1alpha1.Resource("noderecovery"))}
}

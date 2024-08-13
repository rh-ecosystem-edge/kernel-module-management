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

// Code generated by client-gen. DO NOT EDIT.

package v1beta2

import (
	"context"

	v1beta2 "k8s.io/api/apps/v1beta2"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	appsv1beta2 "k8s.io/client-go/applyconfigurations/apps/v1beta2"
	gentype "k8s.io/client-go/gentype"
	scheme "k8s.io/client-go/kubernetes/scheme"
)

// ReplicaSetsGetter has a method to return a ReplicaSetInterface.
// A group's client should implement this interface.
type ReplicaSetsGetter interface {
	ReplicaSets(namespace string) ReplicaSetInterface
}

// ReplicaSetInterface has methods to work with ReplicaSet resources.
type ReplicaSetInterface interface {
	Create(ctx context.Context, replicaSet *v1beta2.ReplicaSet, opts v1.CreateOptions) (*v1beta2.ReplicaSet, error)
	Update(ctx context.Context, replicaSet *v1beta2.ReplicaSet, opts v1.UpdateOptions) (*v1beta2.ReplicaSet, error)
	// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
	UpdateStatus(ctx context.Context, replicaSet *v1beta2.ReplicaSet, opts v1.UpdateOptions) (*v1beta2.ReplicaSet, error)
	Delete(ctx context.Context, name string, opts v1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error
	Get(ctx context.Context, name string, opts v1.GetOptions) (*v1beta2.ReplicaSet, error)
	List(ctx context.Context, opts v1.ListOptions) (*v1beta2.ReplicaSetList, error)
	Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1beta2.ReplicaSet, err error)
	Apply(ctx context.Context, replicaSet *appsv1beta2.ReplicaSetApplyConfiguration, opts v1.ApplyOptions) (result *v1beta2.ReplicaSet, err error)
	// Add a +genclient:noStatus comment above the type to avoid generating ApplyStatus().
	ApplyStatus(ctx context.Context, replicaSet *appsv1beta2.ReplicaSetApplyConfiguration, opts v1.ApplyOptions) (result *v1beta2.ReplicaSet, err error)
	ReplicaSetExpansion
}

// replicaSets implements ReplicaSetInterface
type replicaSets struct {
	*gentype.ClientWithListAndApply[*v1beta2.ReplicaSet, *v1beta2.ReplicaSetList, *appsv1beta2.ReplicaSetApplyConfiguration]
}

// newReplicaSets returns a ReplicaSets
func newReplicaSets(c *AppsV1beta2Client, namespace string) *replicaSets {
	return &replicaSets{
		gentype.NewClientWithListAndApply[*v1beta2.ReplicaSet, *v1beta2.ReplicaSetList, *appsv1beta2.ReplicaSetApplyConfiguration](
			"replicasets",
			c.RESTClient(),
			scheme.ParameterCodec,
			namespace,
			func() *v1beta2.ReplicaSet { return &v1beta2.ReplicaSet{} },
			func() *v1beta2.ReplicaSetList { return &v1beta2.ReplicaSetList{} }),
	}
}

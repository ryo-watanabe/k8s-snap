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

package fake

import (
	v1alpha1 "github.com/ryo-watanabe/k8s-backup/pkg/apis/clusterbackup/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeObjectstoreConfigs implements ObjectstoreConfigInterface
type FakeObjectstoreConfigs struct {
	Fake *FakeClusterbackupV1alpha1
	ns   string
}

var objectstoreconfigsResource = schema.GroupVersionResource{Group: "clusterbackup.ssl", Version: "v1alpha1", Resource: "objectstoreconfigs"}

var objectstoreconfigsKind = schema.GroupVersionKind{Group: "clusterbackup.ssl", Version: "v1alpha1", Kind: "ObjectstoreConfig"}

// Get takes name of the objectstoreConfig, and returns the corresponding objectstoreConfig object, and an error if there is any.
func (c *FakeObjectstoreConfigs) Get(name string, options v1.GetOptions) (result *v1alpha1.ObjectstoreConfig, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(objectstoreconfigsResource, c.ns, name), &v1alpha1.ObjectstoreConfig{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ObjectstoreConfig), err
}

// List takes label and field selectors, and returns the list of ObjectstoreConfigs that match those selectors.
func (c *FakeObjectstoreConfigs) List(opts v1.ListOptions) (result *v1alpha1.ObjectstoreConfigList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(objectstoreconfigsResource, objectstoreconfigsKind, c.ns, opts), &v1alpha1.ObjectstoreConfigList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.ObjectstoreConfigList{ListMeta: obj.(*v1alpha1.ObjectstoreConfigList).ListMeta}
	for _, item := range obj.(*v1alpha1.ObjectstoreConfigList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested objectstoreConfigs.
func (c *FakeObjectstoreConfigs) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(objectstoreconfigsResource, c.ns, opts))

}

// Create takes the representation of a objectstoreConfig and creates it.  Returns the server's representation of the objectstoreConfig, and an error, if there is any.
func (c *FakeObjectstoreConfigs) Create(objectstoreConfig *v1alpha1.ObjectstoreConfig) (result *v1alpha1.ObjectstoreConfig, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(objectstoreconfigsResource, c.ns, objectstoreConfig), &v1alpha1.ObjectstoreConfig{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ObjectstoreConfig), err
}

// Update takes the representation of a objectstoreConfig and updates it. Returns the server's representation of the objectstoreConfig, and an error, if there is any.
func (c *FakeObjectstoreConfigs) Update(objectstoreConfig *v1alpha1.ObjectstoreConfig) (result *v1alpha1.ObjectstoreConfig, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(objectstoreconfigsResource, c.ns, objectstoreConfig), &v1alpha1.ObjectstoreConfig{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ObjectstoreConfig), err
}

// Delete takes name of the objectstoreConfig and deletes it. Returns an error if one occurs.
func (c *FakeObjectstoreConfigs) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(objectstoreconfigsResource, c.ns, name), &v1alpha1.ObjectstoreConfig{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeObjectstoreConfigs) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(objectstoreconfigsResource, c.ns, listOptions)

	_, err := c.Fake.Invokes(action, &v1alpha1.ObjectstoreConfigList{})
	return err
}

// Patch applies the patch and returns the patched objectstoreConfig.
func (c *FakeObjectstoreConfigs) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.ObjectstoreConfig, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(objectstoreconfigsResource, c.ns, name, pt, data, subresources...), &v1alpha1.ObjectstoreConfig{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ObjectstoreConfig), err
}
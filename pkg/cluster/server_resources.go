package cluster

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

// ServerResources holds informations of API resources
type ServerResources struct {
	serverResources []*metav1.APIResourceList
	resourceNames   map[schema.GroupVersionKind]string
}

func matchVerbs(groupVersion string, r *metav1.APIResource) bool {
	return discovery.SupportsAllVerbs{Verbs: []string{"list", "create", "get", "delete"}}.Match(groupVersion, r)
}

func newServerResources(sr []*metav1.APIResourceList) *ServerResources {
	return &ServerResources{
		serverResources: discovery.FilteredBy(discovery.ResourcePredicateFunc(matchVerbs), sr),
		resourceNames:   make(map[schema.GroupVersionKind]string),
	}
}

// GetResources returns server resources struct
func (sr *ServerResources) GetResources() []*metav1.APIResourceList {
	return sr.serverResources
}

// ResourceName get resource string from GroupVersionKind
func (sr *ServerResources) ResourceName(gvk schema.GroupVersionKind) (string, error) {
	if name, ok := sr.resourceNames[gvk]; ok {
		return name, nil
	}
	for _, resourceGroup := range sr.serverResources {
		gv, err := schema.ParseGroupVersion(resourceGroup.GroupVersion)
		if err != nil {
			return "", fmt.Errorf("unable to parse GroupVersion %s : %s", resourceGroup.GroupVersion, err.Error())
		}
		for _, resource := range resourceGroup.APIResources {
			sr.resourceNames[gv.WithKind(resource.Kind)] = resource.Name
		}
	}
	if name, ok := sr.resourceNames[gvk]; ok {
		return name, nil
	}
	return "", fmt.Errorf("unable to find %s in server resources", gvk)
}

// ResourcePath returns an API path of the resource
func (sr *ServerResources) ResourcePath(item *unstructured.Unstructured) (string, error) {
	path := "/api/v1"
	apiversion := item.GetAPIVersion()
	if apiversion != "v1" {
		path = "/apis/" + apiversion
	}
	namespace := item.GetNamespace()
	if namespace != "" {
		path += "/namespaces/" + namespace
	}
	gv, err := schema.ParseGroupVersion(apiversion)
	if err != nil {
		return "", fmt.Errorf("unable to parse GroupVersion %s : %s", apiversion, err.Error())
	}
	resourceName, err := sr.ResourceName(gv.WithKind(item.GetKind()))
	if err != nil {
		return "", err
	}
	return path + "/" + resourceName + "/" + item.GetName(), nil
}

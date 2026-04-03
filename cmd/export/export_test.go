package export

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestGetFilePath(t *testing.T) {
	testCases := []struct {
		Name         string
		Kind         string
		ApiVersion   string
		NameSpace    string
		ResourceName string
		Expected     string
	}{
		{
			Name:         "Namespaced resource: defualt Namespace",
			Kind:         "ConfigMap",
			ApiVersion:   "v1",
			NameSpace:    "default",
			ResourceName: "test-resource",
			Expected:     "ConfigMap__v1_default_test-resource.yaml",
		},
		{
			Name:         "clusterScroped resource: empty namespace",
			Kind:         "ConfigMap",
			ApiVersion:   "v1",
			ResourceName: "test-resource",
			Expected:     "ConfigMap__v1_clusterscoped_test-resource.yaml",
		},
		{
			Name:         "Namespaced resource: defualt Namespace",
			Kind:         "Deployment",
			ApiVersion:   "apps/v1",
			NameSpace:    "default",
			ResourceName: "test-app",
			Expected:     "Deployment_apps_v1_default_test-app.yaml",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			obj := unstructured.Unstructured{}
			obj.SetKind(tc.Kind)
			obj.SetAPIVersion(tc.ApiVersion)
			obj.SetNamespace(tc.NameSpace)
			obj.SetName(tc.ResourceName)
			res := getFilePath(obj)
			if res != tc.Expected {
				t.Errorf("result didnt match the expected\n result: %v \n expected: %v", res, tc.Expected)
			}
		})
	}

}

func TestIsAdmittedResource(t *testing.T) {
	testCases := []struct {
		Name             string
		Kind             string
		Group            string
		Version          string
		ClusterScopeRbac bool
		Namespaced       bool
		Expected         bool
	}{
		{
			Name:             "NameSpaced resouce only",
			Kind:             "ConfigMap",
			Group:            "",
			Version:          "v1",
			ClusterScopeRbac: false,
			Namespaced:       true,
			Expected:         true,
		},
		{
			Name:             "clusterRole without ClusterscopedRbac  flag",
			Kind:             "ClusterRole",
			Group:            "rbac.authorization.k8s.io",
			Version:          "v1",
			ClusterScopeRbac: false,
			Namespaced:       false,
			Expected:         false,
		},
		{
			Name:             "Node with ClusterScopedRbac off",
			Kind:             "Node",
			Group:            "",
			Version:          "v1",
			ClusterScopeRbac: false,
			Namespaced:       false,
			Expected:         false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			gv := schema.GroupVersion{Group: tc.Group, Version: tc.Version}
			resource := metav1.APIResource{Kind: tc.Kind, Namespaced: tc.Namespaced}
			res := isAdmittedResource(tc.ClusterScopeRbac, gv, resource)
			if res != tc.Expected {
				t.Errorf("result didnt match the expected\n result: %v \n expected: %v", res, tc.Expected)
			}
		})
	}

}

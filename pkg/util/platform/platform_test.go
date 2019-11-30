package platform

import (
	"context"
	"testing"

	configapiv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	client "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"
)

var (
	infraObjBase = configapiv1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: configapiv1.InfrastructureSpec{},
		Status: configapiv1.InfrastructureStatus{
			InfrastructureName:   "test-cluster",
			EtcdDiscoveryDomain:  "test-cluster.example.com",
			APIServerURL:         "https://api.test-cluster.example.com:6443",
			APIServerInternalURL: "https://api-int.test-cluster.example.com:6443",
		},
	}

	clusterConfigBase = corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "kube-system",
			Name:      "cluster-config-v1",
		},
	}
)

func TestGetPlatformStatus(t *testing.T) {
	var err error

	var testcases = []struct {
		name           string
		platform       string
		region         string
		useInfraObject bool
	}{
		{
			name:           "us-west-1, use infra object",
			platform:       "AWS",
			region:         "us-west-1",
			useInfraObject: true,
		},
		{
			name:           "ca-central-1, use infra object",
			platform:       "AWS",
			region:         "ca-central-1",
			useInfraObject: true,
		},
		{
			name:           "us-east-2, use configmap",
			platform:       "AWS",
			region:         "us-east-2",
			useInfraObject: false,
		},
	}
	scheme := runtime.NewScheme()
	if err = configapiv1.Install(scheme); err != nil {
		t.Fatalf("unable to create schema: %v", err)
	}
	if err = corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("unable to create schema: %v", err)
	}

	for _, tc := range testcases {
		var err error

		t.Logf("Running scenario %q", tc.name)

		if tc.platform != "AWS" && tc.useInfraObject == false {
			t.Fatalf("error in test case; infrastructure object required for non-AWS platform")
		}

		fc := client.NewFakeClientWithScheme(scheme)

		// create infrastructure object
		infraObj := infraObjBase
		if tc.useInfraObject == true {
			infraObj.Status.PlatformStatus = &configapiv1.PlatformStatus{
				Type: configapiv1.PlatformType(tc.platform),
				AWS: &configapiv1.AWSPlatformStatus{
					Region: tc.region,
				},
			}
		} else {
			//lint:ignore SA1019 ignore deprecation, as this function is specifically designed for backwards compatibility
			//nolint:staticcheck // ref https://github.com/golangci/golangci-lint/issues/741
			infraObj.Status.Platform = configapiv1.PlatformType(tc.platform)
		}
		err = fc.Create(context.TODO(), &infraObj)
		if err != nil {
			t.Fatalf("unable to create fake infratructure object: %v", err)
		}

		// create cluster configmap
		clusterConfig := clusterConfigBase
		var ic installConfig
		ic.Platform.AWS.Region = tc.region
		icYaml, err := yaml.Marshal(ic)
		if err != nil {
			t.Fatalf("unable to marshal yaml: %v", err)
		}
		if clusterConfig.Data == nil {
			clusterConfig.Data = make(map[string]string)
		}
		clusterConfig.Data["install-config"] = string(icYaml)
		err = fc.Create(context.TODO(), &clusterConfig)
		if err != nil {
			t.Fatalf("unable to create fake configmap object: %v", err)
		}

		// Run test and compare
		ps, err := GetPlatformStatus(fc)
		if err != nil {
			t.Errorf("error on retrieving platform status: %v", err)
		}
		if ps.AWS.Region != tc.region {
			t.Errorf("expecting region %s, got %s", tc.region, ps.AWS.Region)
		}
	}
}

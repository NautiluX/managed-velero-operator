package platform

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"

	configapiv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
)

type installConfig struct {
	Platform struct {
		AWS struct {
			Region string `json:"region"`
		} `json:"aws"`
	} `json:"platform"`
}

// GetPlatformStatusClient provides a k8s client that is capable of retrieving
// the items necessary to determine the platform status.
func GetPlatformStatusClient() (client.Client, error) {
	var err error
	scheme := runtime.NewScheme()

	// Set up platform status client
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	// Add OpenShift config apis to scheme
	if err := configapiv1.Install(scheme); err != nil {
		return nil, err
	}

	// Add Core apis to scheme
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, err
	}

	// Create client
	return client.New(cfg, client.Options{Scheme: scheme})
}

// GetPlatformStatus provides a backwards-compatible way to look up platform
// status. AWS is the special case. 4.1 clusters on AWS expose the region config
// only through install-config. New AWS clusters and all other 4.2+ platforms
// are configured via platform status.
func GetPlatformStatus(client client.Client) (*configapiv1.PlatformStatus, error) {
	var err error

	// Retrieve the cluster infrastructure config.
	infra := &configapiv1.Infrastructure{}
	err = client.Get(context.TODO(), types.NamespacedName{Name: "cluster"}, infra)
	if err != nil {
		return nil, err
	}

	if status := infra.Status.PlatformStatus; status != nil {
		// Only AWS needs backwards compatibility with install-config
		if status.Type != configapiv1.AWSPlatformType {
			return status, nil
		}

		// Check whether the cluster config is already migrated
		if status.AWS != nil && len(status.AWS.Region) > 0 {
			return status, nil
		}
	}

	// Otherwise build a platform status from the deprecated install-config
	clusterConfigName := types.NamespacedName{Namespace: "kube-system", Name: "cluster-config-v1"}
	clusterConfig := &corev1.ConfigMap{}
	if err := client.Get(context.TODO(), clusterConfigName, clusterConfig); err != nil {
		return nil, fmt.Errorf("failed to get configmap %s: %v", clusterConfigName, err)
	}
	data, ok := clusterConfig.Data["install-config"]
	if !ok {
		return nil, fmt.Errorf("missing install-config in configmap")
	}
	var ic installConfig
	if err := yaml.Unmarshal([]byte(data), &ic); err != nil {
		return nil, fmt.Errorf("invalid install-config: %v\njson:\n%s", err, data)
	}
	return &configapiv1.PlatformStatus{
		//lint:ignore SA1019 ignore deprecation, as this function is specifically designed for backwards compatibility
		//nolint:staticcheck // ref https://github.com/golangci/golangci-lint/issues/741
		Type: infra.Status.Platform,
		AWS: &configapiv1.AWSPlatformStatus{
			Region: ic.Platform.AWS.Region,
		},
	}, nil
}

// IsPlatformSupported checks if specified platform is in a slice of supported
// platforms
func IsPlatformSupported(platform configapiv1.PlatformType, supportedPlatforms []configapiv1.PlatformType) bool {
	for _, p := range supportedPlatforms {
		if p == platform {
			return true
		}
	}
	return false
}

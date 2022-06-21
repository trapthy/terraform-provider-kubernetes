package provider

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	aggregator "k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset"
)

type KubeClientsets interface {
	MainClientset() (*kubernetes.Clientset, error)
	AggregatorClientset() (*aggregator.Clientset, error)
	DynamicClient() (dynamic.Interface, error)
	DiscoveryClient() (discovery.DiscoveryInterface, error)

	// FIXME: this is not a clientset, and wants to be its own thing
	ConfigData() *schema.ResourceData
}

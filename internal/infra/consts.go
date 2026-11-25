// Package infra holds the local k3s devbox state, config and naming rules.
package infra

const (
	// TraefikClusterIP is the fixed ClusterIP of the ingress controller.
	TraefikClusterIP = "10.43.200.80"
	// DNSClusterIP is the fixed ClusterIP of the cluster DNS for the local domain.
	DNSClusterIP = "10.43.200.53"
	// RegistryClusterIP is the fixed ClusterIP of the in-cluster image registry.
	RegistryClusterIP = "10.43.200.10"
	// RegistryHost is the registry alias (host:port) images are tagged and pulled with.
	RegistryHost = "registry.zenify.internal:5000"
	// PlatformNamespace is the namespace holding platform components.
	PlatformNamespace = "zenify-system"
	// BaseSlug is the slug of the always-on base environment.
	BaseSlug = "base"
	// K3sVersion is the pinned k3s release (measured working on Ubuntu 24.04).
	K3sVersion = "v1.37.2+k3s1"
	// ArgoCDVersion is the pinned ArgoCD release.
	ArgoCDVersion = "v3.6.1"
	// K3sKubeconfig is the kubeconfig k3s writes for the local cluster.
	K3sKubeconfig = "/etc/rancher/k3s/k3s.yaml"
	// BuildkitVersion is the pinned buildkit release.
	BuildkitVersion = "v0.34.2"
)

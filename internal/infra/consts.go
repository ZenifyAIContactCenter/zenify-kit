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
	// BuildkitVersion is the pinned buildkit release.
	BuildkitVersion = "v0.34.2"
)

// Package infra holds the local k3s devbox state, config and naming rules.
package infra

const (
	// TraefikClusterIP is the fixed ClusterIP of the ingress controller.
	TraefikClusterIP = "10.43.200.80"
	// DNSClusterIP is the fixed ClusterIP of the cluster DNS for the local domain.
	DNSClusterIP = "10.43.200.53"
	// RegistryAddr is the in-cluster image registry address.
	RegistryAddr = "10.43.200.10:5000"
	// PlatformNamespace is the namespace holding platform components.
	PlatformNamespace = "zenify-system"
	// BaseSlug is the slug of the always-on base environment.
	BaseSlug = "base"
	// BuildkitVersion is the pinned buildkit release.
	BuildkitVersion = "v0.34.2"
)

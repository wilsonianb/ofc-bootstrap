package types

import (
	"os"
	"strings"
)

const (
	// DefaultFeature filter is for the features which are mandatory
	DefaultFeature = "default"

	// GCPDNS filter enables the creation of secrets for Google Cloud Platform DNS when TLS is enabled
	GCPDNS = "gcp_dns01"
	// DODNS filter enables the creation of secrets for Digital Ocean DNS when TLS is enabled
	DODNS = "do_dns01"
	// Route53DNS filter enables the creation of secrets for Amazon Route53 DNS when TLS is enabled
	Route53DNS = "route53_dns01"
	// CloudflareDNS filter enables the creation of secrets for Cloudflare DNS when TLS is enabled
	CloudflareDNS = "cloudflare_dns01"

	// CloudDNS is the dns_service field in yaml file for Google Cloud Platform
	CloudDNS = "clouddns"
	// DigitalOcean is the dns_service field in yaml file for Digital Ocean
	DigitalOcean = "digitalocean"
	// Route53 is the dns_service field in yaml file for Amazon
	Route53 = "route53"
	// Cloudflare for dns_service
	Cloudflare = "cloudflare"
)

type Plan struct {
	Features             []string                 `yaml:"features,omitempty"`
	Orchestration        string                   `yaml:"orchestration,omitempty"`
	Secrets              []KeyValueNamespaceTuple `yaml:"secrets,omitempty"`
	RootDomain           string                   `yaml:"root_domain,omitempty"`
	TLS                  bool                     `yaml:"tls,omitempty"`
	TLSConfig            TLSConfig                `yaml:"tls_config,omitempty"`
	Ingress              string                   `yaml:"ingress,omitempty"`
	Deployment           Deployment               `yaml:"deployment,omitempty"`
	EnableDockerfileLang bool                     `yaml:"enable_dockerfile_lang,omitempty"`
	ScaleToZero          bool                     `yaml:"scale_to_zero,omitempty"`
	OpenFaaSCloudVersion string                   `yaml:"openfaas_cloud_version,omitempty"`
	NetworkPolicies      bool                     `yaml:"network_policies,omitempty"`
	IngressOperator      bool                     `yaml:"ingress_operator,omitempty"`
	OpenFaaSOperator     bool                     `yaml:"openfaas_operator,omitempty"`
}

// Deployment is the deployment section of YAML concerning
// functions as deployed
type Deployment struct {
	CustomTemplate []string `yaml:"custom_templates,omitempty"`
}

// FormatCustomTemplates are formatted in a CSV format with a space after each comma
func (d Deployment) FormatCustomTemplates() string {
	val := ""
	for _, templateURL := range d.CustomTemplate {
		val = val + templateURL + ", "
	}

	return strings.TrimRight(val, " ,")
}

type KeyValueTuple struct {
	Name  string `yaml:"name,omitempty"`
	Value string `yaml:"value,omitempty"`
}

type FileSecret struct {
	Name      string `yaml:"name,omitempty"`
	ValueFrom string `yaml:"value_from,omitempty"`

	// ValueCommand is a command to execute to generate
	// a secret file specified in ValueFrom
	ValueCommand string `yaml:"value_command,omitempty"`
}

// ExpandValueFrom expands ~ to the home directory of the current user
// kept in the HOME env-var.
func (fs FileSecret) ExpandValueFrom() string {
	value := fs.ValueFrom
	value = strings.Replace(value, "~", os.Getenv("HOME"), -1)
	return value
}

type KeyValueNamespaceTuple struct {
	Name      string          `yaml:"name,omitempty"`
	Literals  []KeyValueTuple `yaml:"literals,omitempty"`
	Namespace string          `yaml:"namespace,omitempty"`
	Files     []FileSecret    `yaml:"files,omitempty"`
	Type      string          `yaml:"type,omitempty"`
	Filters   []string        `yaml:"filters,omitempty"`
}

type TLSConfig struct {
	Email       string `yaml:"email,omitempty"`
	DNSService  string `yaml:"dns_service,omitempty"`
	ProjectID   string `yaml:"project_id,omitempty"`
	IssuerType  string `yaml:"issuer_type,omitempty"`
	Region      string `yaml:"region,omitempty"`
	AccessKeyID string `yaml:"access_key_id,omitempty"`
}

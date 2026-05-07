package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type initTemplate struct {
	files map[string]string
}

func newInitCmd(stdout io.Writer) *cobra.Command {
	var (
		templateName string
		outDir       string
	)
	cmd := &cobra.Command{
		Use:   "init --template <name> --out <dir>",
		Short: "Generate current validating desired-state templates",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().StringVar(&templateName, "template", "", "template name ("+strings.Join(initTemplateNames(), ", ")+")")
	cmd.Flags().StringVar(&outDir, "out", "", "directory to create desired-state YAML files in")
	cmd.RunE = func(_ *cobra.Command, _ []string) error {
		templateName = strings.TrimSpace(templateName)
		outDir = strings.TrimSpace(outDir)
		if templateName == "" {
			return failf(2, "--template is required")
		}
		if outDir == "" {
			return failf(2, "--out is required")
		}
		template, ok := initTemplates[templateName]
		if !ok {
			return failf(2, "unknown template %q (known: %s)", templateName, strings.Join(initTemplateNames(), ", "))
		}
		if err := os.MkdirAll(outDir, 0o700); err != nil {
			return failErr(1, fmt.Errorf("create %s: %w", outDir, err))
		}
		names := make([]string, 0, len(template.files))
		for name := range template.files {
			names = append(names, name)
		}
		sort.Strings(names)
		printTitle(stdout, "Init")
		for _, name := range names {
			path := filepath.Join(outDir, name)
			if _, err := os.Stat(path); err == nil {
				return failf(1, "%s already exists", path)
			} else if !os.IsNotExist(err) {
				return failErr(1, fmt.Errorf("stat %s: %w", path, err))
			}
			if err := os.WriteFile(path, []byte(template.files[name]), 0o644); err != nil {
				return failErr(1, fmt.Errorf("write %s: %w", path, err))
			}
			fmt.Fprintf(stdout, "- %s\n", path)
		}
		return nil
	}
	return cmd
}

func initTemplateNames() []string {
	names := make([]string, 0, len(initTemplates))
	for name := range initTemplates {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

var initTemplates = map[string]initTemplate{
	"libvirt-redfish-hub": {
		files: map[string]string{
			"environment.yaml":                libvirtRedfishHubEnvironmentYAML,
			"provider.yaml":                   libvirtRedfishHubProviderYAML,
			"cluster-infrastructure-hub.yaml": libvirtRedfishHubClusterInfrastructureYAML,
			"ocp-cluster-hub.yaml":            libvirtRedfishHubOCPClusterYAML,
		},
	},
}

const libvirtRedfishHubEnvironmentYAML = `apiVersion: gitups.io/v1alpha1
kind: Environment
metadata:
  name: disconnected-hub
spec:
  baseDomain: disconnected.example.test
  ocpInstall:
    disconnected:
      proxy:
        httpProxy: http://proxy.disconnected.example.test:3128
        httpsProxy: http://proxy.disconnected.example.test:3128
        noProxy:
          - .disconnected.example.test
          - 192.168.130.0/24
      registries:
        mirror:
          url: registry.disconnected.example.test:5000
          credentialsRef:
            name: disconnected-hub-registry
          trustBundleRef:
            name: disconnected-hub-registry-ca
        imageDigestSources:
          - source: quay.io/openshift-release-dev/ocp-release
            mirrors:
              - registry.disconnected.example.test:5000/openshift/release-images
          - source: quay.io/openshift-release-dev/ocp-v4.0-art-dev
            mirrors:
              - registry.disconnected.example.test:5000/openshift/release-images
  secrets:
    pullSecretRef:
      name: openshift-pull-secret
    clusterSSHKeyRef:
      name: cluster-admin-key
  keys:
    libvirt-host-ssh:
      file: ~/.ssh/id_rsa
    cluster-admin-key:
      file: ~/.ssh/id_rsa.pub
    openshift-pull-secret:
      file: ./pull-secret.json
    disconnected-hub-registry-ca:
      generated:
        selfSignedCertificate:
          commonName: registry.disconnected.example.test
    disconnected-hub-registry:
      generated:
        credentials:
          username: admin
    libvirt-redfish-bmc:
      generated:
        credentials:
          username: admin
  openshift:
    release:
      channel: stable-4.21
      version: 4.21.12
  componentImages:
    load-balancer:
      haproxy:
        local: registry.disconnected.example.test:5000/library/haproxy:3.3.8
        public: docker.io/library/haproxy:3.3.8@sha256:f14a1788b56894e7ec7b5cb0ca09dbb959b674cf3c980f92139ec008167d4a91
`

const libvirtRedfishHubProviderYAML = `apiVersion: gitups.io/v1alpha1
kind: InfrastructureProvider
metadata:
  name: libvirt-redfish-provider
spec:
  hosts:
    libvirt-host:
      ssh:
        address: 192.168.10.11
        user: gitups
        keyRef:
          name: libvirt-host-ssh
      capabilities:
        - libvirt
        - hosts-file
        - mirror-registry
  machine:
    libvirt:
      hostRefs:
        - name: libvirt-host
      bmcEmulation:
        enabled: true
        protocol: redfish
        auth:
          credentialRef:
            name: libvirt-redfish-bmc
      machineProfiles:
        sno:
          cpu: 8
          memoryMiB: 22528
          diskGiB: 120
  loadBalancer:
    haProxy:
      hostRef:
        name: libvirt-host
  nameResolution:
    hostsFile:
      hostRefs:
        - name: libvirt-host
  registry:
    mirrorRegistry:
      hostRef:
        name: libvirt-host
      port: 5000
`

const libvirtRedfishHubClusterInfrastructureYAML = `apiVersion: gitups.io/v1alpha1
kind: ClusterInfrastructure
metadata:
  name: hub
spec:
  providerRefs:
    - name: libvirt-redfish-provider
  networks:
    primary:
      cidr: 192.168.130.0/24
      gateway: 192.168.130.1
      dnsServers:
        - 192.168.130.1
      libvirt:
        bridge: vbr-hub
  machines:
    master-0:
      profileRef:
        name: sno
      interfaces:
        primary:
          networkRef:
            name: primary
          ipAddress: 192.168.130.20
          macAddress: 52:54:00:21:11:10
      rootDeviceHints:
        deviceName: /dev/vda
      libvirt:
        hostRef:
          name: libvirt-host
  endpoints:
    api:
      address: 192.168.130.10
    apiInt:
      address: 192.168.130.10
    ingress:
      address: 192.168.130.11
  loadBalancers:
    default:
      endpoints:
        - api
        - apiInt
        - ingress
`

const libvirtRedfishHubOCPClusterYAML = `apiVersion: gitups.io/v1alpha1
kind: OCPCluster
metadata:
  name: hub
spec:
  topology: single-node
  infrastructureRef:
    name: hub
  install:
    method: agent
  networking:
    clusterNetwork:
      - cidr: 10.128.0.0/14
        hostPrefix: 23
    serviceNetwork:
      - 172.30.0.0/16
  nodes:
    master-0:
      role: control-plane
`

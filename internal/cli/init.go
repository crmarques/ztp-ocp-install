package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const (
	initProviderVSphere           = "vsphere"
	initProviderBareMetal         = "bare-metal"
	initProviderEmulatedBareMetal = "emulated-bare-metal"
)

func newInitCmd(stdout io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init <target>",
		Short: "Scaffold workspace material",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(
		newInitWorkspaceCmd(stdout),
		newGitopsInitCmd(),
	)
	showSubcommandFlagsInHelp(cmd)
	return cmd
}

func newInitWorkspaceCmd(stdout io.Writer) *cobra.Command {
	var (
		clusterName string
		provider    string
		stateDir    string
		force       bool
	)
	stateDir = defaultStateDir()
	cmd := &cobra.Command{
		Use:   "workspace --cluster-name <name> --provider <provider>",
		Short: "Create a clusters-bootstrap.git workspace with Gitups input files",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().StringVar(&clusterName, "cluster-name", "", "cluster name to scaffold")
	cmd.Flags().StringVar(&provider, "provider", "", "provider scaffold: vsphere|bare-metal|emulated-bare-metal")
	cmd.Flags().StringVar(&stateDir, "state-dir", stateDir, "generated state directory (env: GITUPS_STATE_DIR)")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing scaffold files for the cluster")
	cmd.RunE = func(_ *cobra.Command, _ []string) error {
		clusterName = strings.TrimSpace(clusterName)
		provider = strings.TrimSpace(provider)
		if clusterName == "" {
			return failf(2, "--cluster-name is required")
		}
		if provider == "" {
			return failf(2, "--provider is required")
		}
		files, err := initRepoFiles(clusterName, provider)
		if err != nil {
			return failErr(2, err)
		}

		repo := bootstrapRepoDir(stateDir)
		gitupsDir := filepath.Join(repo, clusterName, "gitups")
		openshiftDir := filepath.Join(repo, clusterName, "openshift")
		for _, dir := range []string{gitupsDir, openshiftDir} {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return failErr(1, fmt.Errorf("create %s: %w", dir, err))
			}
		}
		if err := initGitRepo(repo); err != nil {
			return failErr(1, err)
		}

		printTitle(stdout, "init workspace")
		for _, item := range files {
			path := filepath.Join(gitupsDir, item.name)
			if _, err := os.Stat(path); err == nil && !force {
				return failf(1, "%s already exists", path)
			} else if err != nil && !os.IsNotExist(err) {
				return failErr(1, fmt.Errorf("stat %s: %w", path, err))
			}
			if err := os.WriteFile(path, []byte(item.body), 0o644); err != nil {
				return failErr(1, fmt.Errorf("write %s: %w", path, err))
			}
			fmt.Fprintf(stdout, "- %s\n", path)
		}
		fmt.Fprintf(stdout, "- %s\n", openshiftDir)
		return nil
	}
	return cmd
}

type initRepoFile struct {
	name string
	body string
}

func initRepoFiles(clusterName, provider string) ([]initRepoFile, error) {
	switch provider {
	case initProviderEmulatedBareMetal:
		return emulatedBareMetalInitRepoFiles(clusterName), nil
	case initProviderBareMetal:
		return bareMetalInitRepoFiles(clusterName), nil
	case initProviderVSphere:
		return vSphereInitRepoFiles(clusterName), nil
	default:
		return nil, fmt.Errorf("unknown provider %q (known: vsphere, bare-metal, emulated-bare-metal)", provider)
	}
}

func initGitRepo(repo string) error {
	if _, err := os.Stat(filepath.Join(repo, ".git")); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", filepath.Join(repo, ".git"), err)
	}
	cmd := exec.Command("git", "init", repo)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git init %s: %w\n%s", repo, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func emulatedBareMetalInitRepoFiles(clusterName string) []initRepoFile {
	providerName := clusterName + "-emulated-bare-metal"
	return []initRepoFile{
		{name: "environment.yaml", body: connectedEnvironmentYAML(clusterName, providerHostSSHKeyYAML()+bmcCredentialsKeyYAML())},
		{name: "provider.yaml", body: fmt.Sprintf(`apiVersion: gitups.io/v1alpha1
kind: InfrastructureProvider
metadata:
  name: %[1]s
spec:
  hosts:
    lab-host:
      ssh:
        address: 192.168.10.11
        user: gitups
        keyRef:
          name: provider-host-ssh
      capabilities:
        - libvirt
        - hosts-file
  machine:
    libvirt:
      hostRefs:
        - name: lab-host
      bmcEmulation:
        enabled: true
        protocol: redfish
        auth:
          credentialRef:
            name: bmc-credentials
      machineProfiles:
        sno:
          cpu: 8
          memoryMiB: 22528
          diskGiB: 120
  loadBalancer:
    haProxy:
      hostRef:
        name: lab-host
  nameResolution:
    hostsFile:
      hostRefs:
        - name: lab-host
`, providerName)},
		{name: "infra.yaml", body: fmt.Sprintf(`apiVersion: gitups.io/v1alpha1
kind: ClusterInfrastructure
metadata:
  name: %[1]s
spec:
  providerRefs:
    - name: %[2]s
  networks:
    primary:
      cidr: 192.168.130.0/24
      gateway: 192.168.130.1
      dnsServers:
        - 192.168.130.1
      libvirt:
        bridge: vbr-%[1]s
  machines:
    master-0:
      profileRef:
        name: sno
      interfaces:
        primary:
          networkRef:
            name: primary
          ipAddress: 192.168.130.20
      rootDeviceHints:
        deviceName: /dev/vda
      libvirt:
        hostRef:
          name: lab-host
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
`, clusterName, providerName)},
		{name: "cluster.yaml", body: ocpClusterYAML(clusterName)},
	}
}

func bareMetalInitRepoFiles(clusterName string) []initRepoFile {
	providerName := clusterName + "-bare-metal"
	return []initRepoFile{
		{name: "environment.yaml", body: connectedEnvironmentYAML(clusterName, bmcCredentialsKeyYAML())},
		{name: "provider.yaml", body: fmt.Sprintf(`apiVersion: gitups.io/v1alpha1
kind: InfrastructureProvider
metadata:
  name: %[1]s
spec:
  machine:
    baremetal:
      bmcProtocol: redfish
`, providerName)},
		{name: "infra.yaml", body: fmt.Sprintf(`apiVersion: gitups.io/v1alpha1
kind: ClusterInfrastructure
metadata:
  name: %[1]s
spec:
  providerRefs:
    - name: %[2]s
  networks:
    primary:
      cidr: 192.168.130.0/24
      gateway: 192.168.130.1
      dnsServers:
        - 192.168.130.1
  machines:
    master-0:
      interfaces:
        primary:
          networkRef:
            name: primary
          ipAddress: 192.168.130.20
          macAddress: 52:54:00:21:11:10
      rootDeviceHints:
        deviceName: /dev/sda
      baremetal:
        bmc:
          address: redfish-virtualmedia+https://bmc-%[1]s-0.example.test/redfish/v1/Systems/1
          credentialRef:
            name: bmc-credentials
          disableCertificateVerification: true
  endpoints:
    api:
      address: 192.168.130.10
    apiInt:
      address: 192.168.130.10
    ingress:
      address: 192.168.130.11
`, clusterName, providerName)},
		{name: "cluster.yaml", body: ocpClusterYAML(clusterName)},
	}
}

func vSphereInitRepoFiles(clusterName string) []initRepoFile {
	providerName := clusterName + "-vsphere"
	return []initRepoFile{
		{name: "environment.yaml", body: connectedEnvironmentYAML(clusterName, vCenterCredentialsKeyYAML())},
		{name: "provider.yaml", body: fmt.Sprintf(`apiVersion: gitups.io/v1alpha1
kind: InfrastructureProvider
metadata:
  name: %[1]s
spec:
  machine:
    vsphere:
      vCenterRef:
        name: vcenter-credentials
      datacenter: dc1
      cluster: cluster1
`, providerName)},
		{name: "infra.yaml", body: fmt.Sprintf(`apiVersion: gitups.io/v1alpha1
kind: ClusterInfrastructure
metadata:
  name: %[1]s
spec:
  providerRefs:
    - name: %[2]s
  networks:
    primary:
      cidr: 192.168.130.0/24
      gateway: 192.168.130.1
      dnsServers:
        - 192.168.130.1
      vsphere:
        portgroup: ocp-install
  machines:
    master-0:
      interfaces:
        primary:
          networkRef:
            name: primary
          ipAddress: 192.168.130.20
          macAddress: 52:54:00:21:11:10
      rootDeviceHints:
        deviceName: /dev/sda
      vsphere:
        datastore: datastore1
        folder: /Gitups/%[1]s
        template: rhcos
  endpoints:
    api:
      address: 192.168.130.10
    apiInt:
      address: 192.168.130.10
    ingress:
      address: 192.168.130.11
`, clusterName, providerName)},
		{name: "cluster.yaml", body: ocpClusterYAML(clusterName)},
	}
}

func connectedEnvironmentYAML(name string, extraKeys string) string {
	return fmt.Sprintf(`apiVersion: gitups.io/v1alpha1
kind: Environment
metadata:
  name: %[1]s
spec:
  baseDomain: example.test
  ocpInstall:
    connected: {}
  secrets:
    pullSecretRef:
      name: openshift-pull-secret
    clusterSSHKeyRef:
      name: cluster-admin-key
  keys:
    cluster-admin-key:
      file: ~/.ssh/id_rsa.pub
    openshift-pull-secret:
      file: ./pull-secret.json
%s
  openshift:
    release:
      channel: stable-4.21
      version: 4.21.12
`, name, extraKeys)
}

func providerHostSSHKeyYAML() string {
	return `    provider-host-ssh:
      file: ~/.ssh/id_rsa
`
}

func bmcCredentialsKeyYAML() string {
	return `    bmc-credentials:
      generated:
        credentials:
          username: admin
`
}

func vCenterCredentialsKeyYAML() string {
	return `    vcenter-credentials:
      file: ./vcenter-credentials
`
}

func ocpClusterYAML(clusterName string) string {
	return fmt.Sprintf(`apiVersion: gitups.io/v1alpha1
kind: OCPCluster
metadata:
  name: %[1]s
spec:
  role: managed
  topology: single-node
  infrastructureRef:
    name: %[1]s
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
`, clusterName)
}

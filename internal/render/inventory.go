package render

import (
	"fmt"
	"strings"

	"github.com/crmarques/ztp-ocp-install-lab/api/v1alpha1"
)

type InventoryFile struct {
	All InventoryAll `yaml:"all" json:"all"`
}

type InventoryAll struct {
	Vars     map[string]string         `yaml:"vars" json:"vars"`
	Children map[string]InventoryGroup `yaml:"children" json:"children"`
}

type InventoryGroup struct {
	Hosts map[string]InventoryHost `yaml:"hosts" json:"hosts"`
}

type InventoryHost struct {
	AnsibleHost              string `yaml:"ansible_host" json:"ansible_host"`
	AnsibleConnection        string `yaml:"ansible_connection,omitempty" json:"ansible_connection,omitempty"`
	AnsibleUser              string `yaml:"ansible_user" json:"ansible_user"`
	GitupsProviderName       string `yaml:"gitups_provider_name,omitempty" json:"gitups_provider_name,omitempty"`
	GitupsClusterName        string `yaml:"gitups_cluster_name,omitempty" json:"gitups_cluster_name,omitempty"`
	GitupsHostName           string `yaml:"gitups_host_name" json:"gitups_host_name"`
	AnsibleSSHPrivateKeyFile string `yaml:"ansible_ssh_private_key_file" json:"ansible_ssh_private_key_file"`
}

func Inventory(state v1alpha1.State, secretsDir string) InventoryFile {
	groups := map[string]InventoryGroup{
		"gitups_infra_hosts": {
			Hosts: map[string]InventoryHost{},
		},
		"gitups_provider_hosts": {
			Hosts: map[string]InventoryHost{},
		},
		"gitups_ocp_hosts": {
			Hosts: map[string]InventoryHost{},
		},
	}
	providers := providerIndex(state.InfrastructureProviders)
	env := primaryEnvironment(state)
	for _, provider := range state.InfrastructureProviders {
		hostNames := sortedKeys(provider.Spec.Hosts)
		for _, hostName := range hostNames {
			host := provider.Spec.Hosts[hostName]
			if host.SSH == nil {
				continue
			}
			inventoryName := fmt.Sprintf("%s-%s", provider.Metadata.Name, hostName)
			groups["gitups_provider_hosts"].Hosts[inventoryName] = InventoryHost{
				AnsibleHost:              host.SSH.Address,
				AnsibleConnection:        ansibleConnection(host.SSH.Address),
				AnsibleUser:              host.SSH.User,
				GitupsProviderName:       provider.Metadata.Name,
				GitupsHostName:           hostName,
				AnsibleSSHPrivateKeyFile: resolvedSecretPath(host.SSH.KeyRef.Name, secretsDir, env),
			}
		}
	}
	for _, item := range state.ClusterInfrastructures {
		closure, _ := v1alpha1.BuildProviderClosure(item, providers)
		if closure.Machine == nil || closure.Machine.Libvirt == nil {
			continue
		}
		for _, ref := range closure.Machine.Libvirt.HostRefs {
			hostName := ref.Name
			host, ok := closure.Hosts[hostName]
			if !ok {
				continue
			}
			if host.SSH == nil {
				continue
			}
			inventoryName := fmt.Sprintf("%s-%s", item.Metadata.Name, hostName)
			inventoryHost := InventoryHost{
				AnsibleHost:              host.SSH.Address,
				AnsibleConnection:        ansibleConnection(host.SSH.Address),
				AnsibleUser:              host.SSH.User,
				GitupsProviderName:       closure.MachineProviderName,
				GitupsClusterName:        item.Metadata.Name,
				GitupsHostName:           hostName,
				AnsibleSSHPrivateKeyFile: resolvedSecretPath(host.SSH.KeyRef.Name, secretsDir, env),
			}
			groups["gitups_infra_hosts"].Hosts[inventoryName] = inventoryHost
			groups["gitups_ocp_hosts"].Hosts[inventoryName] = inventoryHost
		}
	}
	return InventoryFile{
		All: InventoryAll{
			Vars: map[string]string{
				"gitups_managed_by": "gitups",
			},
			Children: groups,
		},
	}
}

func ansibleConnection(address string) string {
	switch strings.ToLower(strings.TrimSpace(address)) {
	case "localhost", "127.0.0.1", "::1":
		return "local"
	default:
		return ""
	}
}

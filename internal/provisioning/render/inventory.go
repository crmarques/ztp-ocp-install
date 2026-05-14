package render

import (
	"fmt"

	"github.com/crmarques/bootwright/api/v1alpha1"
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
	AnsibleUser              string `yaml:"ansible_user" json:"ansible_user"`
	BootwrightProviderName   string `yaml:"bootwright_provider_name,omitempty" json:"bootwright_provider_name,omitempty"`
	BootwrightClusterName    string `yaml:"bootwright_cluster_name,omitempty" json:"bootwright_cluster_name,omitempty"`
	BootwrightHostName       string `yaml:"bootwright_host_name" json:"bootwright_host_name"`
	AnsibleSSHPrivateKeyFile string `yaml:"ansible_ssh_private_key_file" json:"ansible_ssh_private_key_file"`
}

func Inventory(state v1alpha1.State, secretsDir string) InventoryFile {
	groups := map[string]InventoryGroup{
		"bootwright_infra_hosts": {
			Hosts: map[string]InventoryHost{},
		},
		"bootwright_provider_hosts": {
			Hosts: map[string]InventoryHost{},
		},
		"bootwright_ocp_hosts": {
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
			groups["bootwright_provider_hosts"].Hosts[inventoryName] = InventoryHost{
				AnsibleHost:              host.SSH.Address,
				AnsibleUser:              host.SSH.User,
				BootwrightProviderName:   provider.Metadata.Name,
				BootwrightHostName:       hostName,
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
				AnsibleUser:              host.SSH.User,
				BootwrightProviderName:   closure.MachineProviderName,
				BootwrightClusterName:    item.Metadata.Name,
				BootwrightHostName:       hostName,
				AnsibleSSHPrivateKeyFile: resolvedSecretPath(host.SSH.KeyRef.Name, secretsDir, env),
			}
			groups["bootwright_infra_hosts"].Hosts[inventoryName] = inventoryHost
			groups["bootwright_ocp_hosts"].Hosts[inventoryName] = inventoryHost
		}
	}
	return InventoryFile{
		All: InventoryAll{
			Vars: map[string]string{
				"bootwright_managed_by": "bootwright",
			},
			Children: groups,
		},
	}
}

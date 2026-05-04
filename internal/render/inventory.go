package render

import (
	"fmt"

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
	AnsibleHost        string `yaml:"ansible_host" json:"ansible_host"`
	AnsibleConnection  string `yaml:"ansible_connection,omitempty" json:"ansible_connection,omitempty"`
	AnsibleUser        string `yaml:"ansible_user" json:"ansible_user"`
	GitupsProviderName string `yaml:"gitups_provider_name,omitempty" json:"gitups_provider_name,omitempty"`
	GitupsClusterName  string `yaml:"gitups_cluster_name,omitempty" json:"gitups_cluster_name,omitempty"`
	GitupsClusterRole  string `yaml:"gitups_cluster_role,omitempty" json:"gitups_cluster_role,omitempty"`
	GitupsHostName     string `yaml:"gitups_host_name" json:"gitups_host_name"`
	GitupsSSHKeyRef    string `yaml:"gitups_ssh_key_ref" json:"gitups_ssh_key_ref"`
}

func Inventory(state v1alpha1.State) InventoryFile {
	groups := map[string]InventoryGroup{
		"gitups_infra_hosts": {
			Hosts: map[string]InventoryHost{},
		},
		"gitups_provider_hosts": {
			Hosts: map[string]InventoryHost{},
		},
		"gitups_hub_hosts": {
			Hosts: map[string]InventoryHost{},
		},
		"gitups_managed_hosts": {
			Hosts: map[string]InventoryHost{},
		},
	}
	providers := providerIndex(state.InfrastructureProviders)
	for _, provider := range state.InfrastructureProviders {
		if provider.Spec.QemuKVM == nil {
			continue
		}
		hostNames := sortedKeys(provider.Spec.QemuKVM.Hosts)
		for _, hostName := range hostNames {
			host := provider.Spec.QemuKVM.Hosts[hostName]
			inventoryName := fmt.Sprintf("%s-%s", provider.Metadata.Name, hostName)
			groups["gitups_provider_hosts"].Hosts[inventoryName] = InventoryHost{
				AnsibleHost:        host.Address,
				AnsibleConnection:  ansibleConnection(host.Address),
				AnsibleUser:        host.User,
				GitupsProviderName: provider.Metadata.Name,
				GitupsHostName:     hostName,
				GitupsSSHKeyRef:    host.SSHKeyRef.Name,
			}
		}
	}
	ocpByInfra := ocpByInfrastructure(state.OCPClusters)
	for _, item := range state.ClusterInfrastructures {
		provider := providers[item.Spec.ProviderRef.Name]
		if provider.Spec.QemuKVM == nil {
			continue
		}
		ocp := ocpByInfra[item.Metadata.Name]
		roleGroup := "gitups_managed_hosts"
		if ocp.Spec.Role == v1alpha1.OCPRoleHub {
			roleGroup = "gitups_hub_hosts"
		}
		hostNames := sortedKeys(provider.Spec.QemuKVM.Hosts)
		for _, hostName := range hostNames {
			host := provider.Spec.QemuKVM.Hosts[hostName]
			inventoryName := fmt.Sprintf("%s-%s", item.Metadata.Name, hostName)
			inventoryHost := InventoryHost{
				AnsibleHost:        host.Address,
				AnsibleConnection:  ansibleConnection(host.Address),
				AnsibleUser:        host.User,
				GitupsProviderName: provider.Metadata.Name,
				GitupsClusterName:  item.Metadata.Name,
				GitupsClusterRole:  ocp.Spec.Role,
				GitupsHostName:     hostName,
				GitupsSSHKeyRef:    host.SSHKeyRef.Name,
			}
			groups["gitups_infra_hosts"].Hosts[inventoryName] = inventoryHost
			groups[roleGroup].Hosts[inventoryName] = inventoryHost
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
	switch address {
	case "localhost", "127.0.0.1", "::1":
		return "local"
	default:
		return ""
	}
}

package infra

import (
	"fmt"
	"strings"

	"github.com/crmarques/gitups/api/v1alpha1"
)

func validateOCPClusters(state v1alpha1.State) []string {
	var errs []string
	seen := map[string]bool{}
	infraIndex := clusterInfraIndex(state.ClusterInfrastructures)
	for _, ocp := range state.OCPClusters {
		if e := validateName("OCPCluster", ocp.Metadata.Name); e != "" {
			errs = append(errs, e)
			continue
		}
		if seen[ocp.Metadata.Name] {
			errs = append(errs, fmt.Sprintf("duplicate OCPCluster %q", ocp.Metadata.Name))
		}
		seen[ocp.Metadata.Name] = true
		switch ocp.Spec.Role {
		case "", v1alpha1.OCPClusterRoleHub, v1alpha1.OCPClusterRoleManaged:
		default:
			errs = append(errs, fmt.Sprintf("OCPCluster/%s spec.role %q must be hub or managed", ocp.Metadata.Name, ocp.Spec.Role))
		}
		switch ocp.Spec.Topology {
		case "", v1alpha1.OCPTopologySingleNode, v1alpha1.OCPTopologyMultiNode:
		default:
			errs = append(errs, fmt.Sprintf("OCPCluster/%s spec.topology %q must be single-node or multi-node", ocp.Metadata.Name, ocp.Spec.Topology))
		}
		if ocp.Spec.Install.Method != "" && ocp.Spec.Install.Method != v1alpha1.OCPInstallMethodAgent {
			errs = append(errs, fmt.Sprintf("OCPCluster/%s spec.install.method %q must be %q", ocp.Metadata.Name, ocp.Spec.Install.Method, v1alpha1.OCPInstallMethodAgent))
		}
		ci, ok := infraIndex[ocp.Spec.InfrastructureRef.Name]
		if !ok {
			errs = append(errs, fmt.Sprintf("OCPCluster/%s spec.infrastructureRef %q does not match any ClusterInfrastructure", ocp.Metadata.Name, ocp.Spec.InfrastructureRef.Name))
			continue
		}
		errs = append(errs, validateNodes(ocp, ci)...)
		errs = append(errs, validateInstallOverrides(ocp)...)
	}
	return errs
}

func validateNodes(ocp v1alpha1.OCPCluster, ci v1alpha1.ClusterInfrastructure) []string {
	var errs []string
	if len(ocp.Spec.Nodes) == 0 {
		errs = append(errs, fmt.Sprintf("OCPCluster/%s spec.nodes is required", ocp.Metadata.Name))
		return errs
	}
	control := 0
	worker := 0
	for nodeName, node := range ocp.Spec.Nodes {
		switch node.Role {
		case v1alpha1.NodeRoleControlPlane:
			control++
		case v1alpha1.NodeRoleWorker:
			worker++
		default:
			errs = append(errs, fmt.Sprintf("OCPCluster/%s nodes[%s].role %q must be control-plane or worker", ocp.Metadata.Name, nodeName, node.Role))
		}
		if node.MachineRef == nil || node.MachineRef.Name == "" {
			errs = append(errs, fmt.Sprintf("OCPCluster/%s nodes[%s].machineRef.name unresolved after normalization", ocp.Metadata.Name, nodeName))
			continue
		}
		if _, ok := ci.Spec.Machines[node.MachineRef.Name]; !ok {
			errs = append(errs, fmt.Sprintf("OCPCluster/%s nodes[%s].machineRef %q not defined on ClusterInfrastructure/%s", ocp.Metadata.Name, nodeName, node.MachineRef.Name, ci.Metadata.Name))
		}
	}
	if ocp.Spec.Topology == v1alpha1.OCPTopologySingleNode {
		if control != 1 || worker != 0 {
			errs = append(errs, fmt.Sprintf("OCPCluster/%s topology=single-node requires exactly 1 control-plane and 0 workers (got %d/%d)", ocp.Metadata.Name, control, worker))
		}
	}
	return errs
}

var installOverrideForbiddenKeys = map[string]bool{
	"apiVersion":            true,
	"metadata":              true,
	"baseDomain":            true,
	"pullSecret":            true,
	"sshKey":                true,
	"additionalTrustBundle": true,
	"controlPlane":          true,
	"compute":               true,
	"imageDigestSources":    true,
}

var installOverrideForbiddenNestedPaths = []string{
	"networking.machineNetwork",
	"platform.baremetal.apiVIPs",
	"platform.baremetal.ingressVIPs",
	"platform.vsphere.apiVIPs",
	"platform.vsphere.ingressVIPs",
}

var agentConfigForbiddenKeys = map[string]bool{
	"apiVersion":           true,
	"kind":                 true,
	"metadata":             true,
	"rendezvousIP":         true,
	"hosts":                true,
	"minimalISO":           true,
	"bootArtifactsBaseURL": true,
}

var sensitiveOverrideKeys = []string{"password", "token", "secret", "apikey", "credential"}

func validateInstallOverrides(ocp v1alpha1.OCPCluster) []string {
	var errs []string
	for k := range ocp.Spec.Install.InstallConfigOverrides {
		if installOverrideForbiddenKeys[k] {
			errs = append(errs, fmt.Sprintf("OCPCluster/%s install.installConfigOverrides[%s] is owned by Gitups and cannot be overridden", ocp.Metadata.Name, k))
		}
	}
	errs = append(errs, validateSensitiveOverridePaths(fmt.Sprintf("OCPCluster/%s install.installConfigOverrides", ocp.Metadata.Name), ocp.Spec.Install.InstallConfigOverrides)...)
	for _, path := range installOverrideForbiddenNestedPaths {
		if hasNestedKey(ocp.Spec.Install.InstallConfigOverrides, path) {
			errs = append(errs, fmt.Sprintf("OCPCluster/%s install.installConfigOverrides[%s] is owned by Gitups and cannot be overridden", ocp.Metadata.Name, path))
		}
	}
	for k := range ocp.Spec.Install.AgentConfigOverrides {
		if agentConfigForbiddenKeys[k] {
			errs = append(errs, fmt.Sprintf("OCPCluster/%s install.agentConfigOverrides[%s] is owned by Gitups and cannot be overridden", ocp.Metadata.Name, k))
		}
	}
	errs = append(errs, validateSensitiveOverridePaths(fmt.Sprintf("OCPCluster/%s install.agentConfigOverrides", ocp.Metadata.Name), ocp.Spec.Install.AgentConfigOverrides)...)
	for _, src := range ocp.Spec.Install.ImageDigestSources {
		errs = append(errs, validateImageDigestSource(fmt.Sprintf("OCPCluster/%s install", ocp.Metadata.Name), src)...)
	}
	if ocp.Spec.Install.PullSecretRef.Name == "" {
		errs = append(errs, fmt.Sprintf("OCPCluster/%s install.pullSecretRef.name is required (inheritable from Environment)", ocp.Metadata.Name))
	}
	if ocp.Spec.Install.SSHKeyRef.Name == "" {
		errs = append(errs, fmt.Sprintf("OCPCluster/%s install.sshKeyRef.name is required (inheritable from Environment)", ocp.Metadata.Name))
	}
	if ocp.Spec.Install.BaseDomain == "" {
		errs = append(errs, fmt.Sprintf("OCPCluster/%s install.baseDomain is required (inheritable from Environment)", ocp.Metadata.Name))
	}
	return errs
}

func validateSensitiveOverridePaths(owner string, value any) []string {
	var errs []string
	var walk func(path string, current any)
	walk = func(path string, current any) {
		switch typed := current.(type) {
		case map[string]any:
			for key, value := range typed {
				next := key
				if path != "" {
					next = path + "." + key
				}
				lower := strings.ToLower(key)
				for _, sub := range sensitiveOverrideKeys {
					if strings.Contains(lower, sub) {
						errs = append(errs, fmt.Sprintf("%s[%s] looks like a sensitive value; use SecretRef instead", owner, next))
						break
					}
				}
				walk(next, value)
			}
		case []any:
			for i, value := range typed {
				next := fmt.Sprintf("[%d]", i)
				if path != "" {
					next = fmt.Sprintf("%s[%d]", path, i)
				}
				walk(next, value)
			}
		}
	}
	walk("", value)
	return errs
}

func hasNestedKey(m map[string]any, path string) bool {
	parts := strings.Split(path, ".")
	var cursor any = m
	for _, key := range parts {
		current, ok := cursor.(map[string]any)
		if !ok {
			return false
		}
		next, ok := current[key]
		if !ok {
			return false
		}
		cursor = next
	}
	return true
}

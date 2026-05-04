package render

import "github.com/crmarques/ztp-ocp-install-lab/api/v1alpha1"

type InfrastructureState struct {
	APIVersion string                  `yaml:"apiVersion" json:"apiVersion"`
	Kind       string                  `yaml:"kind" json:"kind"`
	Metadata   v1alpha1.Metadata       `yaml:"metadata" json:"metadata"`
	Spec       InfrastructureStateSpec `yaml:"spec" json:"spec"`
}

type InfrastructureStateSpec struct {
	Environments            []v1alpha1.Environment            `yaml:"environments,omitempty" json:"environments,omitempty"`
	InfrastructureProviders []v1alpha1.InfrastructureProvider `yaml:"infrastructureProviders" json:"infrastructureProviders"`
	ClusterInfrastructures  []v1alpha1.ClusterInfrastructure  `yaml:"clusterInfrastructures" json:"clusterInfrastructures"`
	OCPClusters             []v1alpha1.OCPCluster             `yaml:"ocpClusters" json:"ocpClusters"`
}

func EffectiveState(state v1alpha1.State) InfrastructureState {
	environments := append([]v1alpha1.Environment(nil), state.Environments...)
	for i := range environments {
		environments[i].SourcePath = ""
	}
	providers := append([]v1alpha1.InfrastructureProvider(nil), state.InfrastructureProviders...)
	for i := range providers {
		providers[i].SourcePath = ""
	}
	infrastructures := append([]v1alpha1.ClusterInfrastructure(nil), state.ClusterInfrastructures...)
	for i := range infrastructures {
		infrastructures[i].SourcePath = ""
	}
	ocpClusters := append([]v1alpha1.OCPCluster(nil), state.OCPClusters...)
	for i := range ocpClusters {
		ocpClusters[i].SourcePath = ""
	}
	return InfrastructureState{
		APIVersion: v1alpha1.APIVersion,
		Kind:       v1alpha1.KindInfrastructureState,
		Metadata: v1alpha1.Metadata{
			Name: "infrastructure",
		},
		Spec: InfrastructureStateSpec{
			Environments:            environments,
			InfrastructureProviders: providers,
			ClusterInfrastructures:  infrastructures,
			OCPClusters:             ocpClusters,
		},
	}
}

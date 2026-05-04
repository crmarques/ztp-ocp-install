package render

import "github.com/crmarques/ztp-ocp-install-lab/api/v1alpha1"

type GitupsLock struct {
	APIVersion string         `yaml:"apiVersion" json:"apiVersion"`
	Kind       string         `yaml:"kind" json:"kind"`
	Metadata   LockMetadata   `yaml:"metadata" json:"metadata"`
	Spec       GitupsLockSpec `yaml:"spec" json:"spec"`
}

type LockMetadata struct {
	Name string `yaml:"name" json:"name"`
}

type GitupsLockSpec struct {
	Components []ComponentPin `yaml:"components" json:"components"`
}

func Lock(state v1alpha1.State) GitupsLock {
	return GitupsLock{
		APIVersion: v1alpha1.APIVersion,
		Kind:       v1alpha1.KindGitupsLock,
		Metadata: LockMetadata{
			Name: "infrastructure",
		},
		Spec: GitupsLockSpec{
			Components: ComponentPins(state),
		},
	}
}

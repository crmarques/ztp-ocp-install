package render

import "github.com/crmarques/bootwright/api/v1alpha1"

type BootwrightLock struct {
	APIVersion string             `yaml:"apiVersion" json:"apiVersion"`
	Kind       string             `yaml:"kind" json:"kind"`
	Metadata   LockMetadata       `yaml:"metadata" json:"metadata"`
	Spec       BootwrightLockSpec `yaml:"spec" json:"spec"`
}

type LockMetadata struct {
	Name string `yaml:"name" json:"name"`
}

type BootwrightLockSpec struct {
	Components []ComponentPin `yaml:"components" json:"components"`
}

func Lock(state v1alpha1.State) BootwrightLock {
	return BootwrightLock{
		APIVersion: v1alpha1.APIVersion,
		Kind:       v1alpha1.KindBootwrightLock,
		Metadata: LockMetadata{
			Name: "infrastructure",
		},
		Spec: BootwrightLockSpec{
			Components: ComponentPins(state),
		},
	}
}

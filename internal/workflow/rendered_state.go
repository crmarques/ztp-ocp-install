package workflow

import (
	"github.com/crmarques/gitups/internal/ansible"
	"github.com/crmarques/gitups/internal/orchestrate/provisioning"
	"github.com/crmarques/gitups/internal/provisioning/render"
)

// RenderedState is the value-object contract between the render and
// orchestrate stages. It carries every path the orchestrate side reads
// (inventory, vars, installer assets, artifacts root) plus the runtime
// context that wires Ansible to those paths (bundle/secrets/host state
// dirs). render.Result lives inside it; RenderedState adds the
// orchestration context render.Result alone cannot know.
//
// Callers should construct one RenderedState at the end of the render
// stage and pass it into NewRunSpec for each subsequent playbook
// invocation. The struct is immutable by convention — fields are not
// mutated after construction, only read.
type RenderedState struct {
	Render       render.Result
	BundleDir    string
	StateDir     string
	SecretsDir   string
	HostStateDir string
}

// NewRunSpec builds an ansible.RunSpec for one playbook invocation
// against this rendered state. Equivalent to constructing a
// provisioning.RunSpecConfig manually, but centralises the binding so
// callers do not re-derive paths.
func (s RenderedState) NewRunSpec(executable, playbook, limit, artifactsDir string, extras []string, check, askBecomePass bool) (ansible.RunSpec, error) {
	return provisioning.NewRunSpec(provisioning.RunSpecConfig{
		Executable:    executable,
		BundleDir:     s.BundleDir,
		StateDir:      s.StateDir,
		SecretsDir:    s.SecretsDir,
		HostStateDir:  s.HostStateDir,
		InventoryPath: s.Render.InventoryPath,
		VarsPath:      s.Render.VarsPath,
		Playbook:      playbook,
		Limit:         limit,
		ExtraVarPairs: extras,
		ArtifactsDir:  artifactsDir,
		Check:         check,
		AskBecomePass: askBecomePass,
	})
}

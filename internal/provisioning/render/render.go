package render

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/crmarques/bootwright/api/v1alpha1"
	"go.yaml.in/yaml/v3"
)

type Result struct {
	EffectiveStatePath string
	LockPath           string
	InventoryPath      string
	VarsPath           string
	ArtifactsDir       string
	InstallerAssets    []InstallerAsset
}

func All(stateDir, secretsDir string, state v1alpha1.State) (Result, error) {
	result := Result{
		EffectiveStatePath: filepath.Join(stateDir, "effective-state.yaml"),
		LockPath:           filepath.Join(stateDir, "bootwright.lock.yaml"),
		InventoryPath:      filepath.Join(stateDir, "ansible", "inventory.yaml"),
		VarsPath:           filepath.Join(stateDir, "ansible", "vars.yaml"),
		ArtifactsDir:       filepath.Join(stateDir, "ansible", "artifacts"),
		InstallerAssets:    InstallerAssets(stateDir, state),
	}
	for _, dir := range []string{
		stateDir,
		filepath.Dir(result.InventoryPath),
		result.ArtifactsDir,
	} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return result, fmt.Errorf("create %s: %w", dir, err)
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return result, fmt.Errorf("chmod %s: %w", dir, err)
		}
	}
	for _, asset := range result.InstallerAssets {
		if err := os.MkdirAll(asset.Dir, 0o700); err != nil {
			return result, fmt.Errorf("create %s: %w", asset.Dir, err)
		}
		if err := os.Chmod(asset.Dir, 0o700); err != nil {
			return result, fmt.Errorf("chmod %s: %w", asset.Dir, err)
		}
	}
	writes := []struct {
		path  string
		value any
	}{
		{path: result.EffectiveStatePath, value: EffectiveState(state)},
		{path: result.LockPath, value: Lock(state)},
		{path: result.InventoryPath, value: Inventory(state, secretsDir)},
		{path: result.VarsPath, value: Vars(state, secretsDir)},
	}
	for _, write := range writes {
		if err := writeYAML(write.path, write.value); err != nil {
			return result, err
		}
	}
	for _, ocp := range state.OCPClusters {
		asset := installerAssetFor(result.InstallerAssets, ocp.Metadata.Name)
		installConfig, err := InstallerConfig(state, ocp)
		if err != nil {
			return result, err
		}
		if err := writeYAML(asset.InstallConfigPath, installConfig); err != nil {
			return result, err
		}
		agentConfig, err := AgentConfig(state, ocp)
		if err != nil {
			return result, err
		}
		if err := writeYAML(asset.AgentConfigPath, agentConfig); err != nil {
			return result, err
		}
	}
	return result, nil
}

// ResolveInstaller writes install-config.yaml / agent-config.yaml under each
// cluster's runtime installer directory (state-dir/runtime/<cluster>/installer/)
// with secrets inlined from secretsDir. Returned paths contain real credentials
// and live outside the bootstrap repo so the GitOps-publishable tree stays
// secret-free; the safe placeholder copies under
// state-dir/git-repos/clusters-bootstrap/<cluster>/openshift/ stay untouched.
func ResolveInstaller(stateDir, secretsDir string, state v1alpha1.State) (Result, error) {
	result := Result{InstallerAssets: InstallerAssets(stateDir, state)}
	for _, ocp := range state.OCPClusters {
		asset := installerAssetFor(result.InstallerAssets, ocp.Metadata.Name)
		secrets, err := LoadInstallerSecrets(state, ocp, secretsDir)
		if err != nil {
			return result, err
		}
		if err := os.MkdirAll(asset.WorkDir, 0o700); err != nil {
			return result, fmt.Errorf("create %s: %w", asset.WorkDir, err)
		}
		if err := os.Chmod(asset.WorkDir, 0o700); err != nil {
			return result, fmt.Errorf("chmod %s: %w", asset.WorkDir, err)
		}
		installConfig, err := InstallerConfigWithSecrets(state, ocp, secrets)
		if err != nil {
			return result, err
		}
		if err := writeYAML(asset.EffectiveInstallConfigPath, installConfig); err != nil {
			return result, err
		}
		agentConfig, err := AgentConfig(state, ocp)
		if err != nil {
			return result, err
		}
		if err := writeYAML(asset.EffectiveAgentConfigPath, agentConfig); err != nil {
			return result, err
		}
	}
	return result, nil
}

func installerAssetFor(assets []InstallerAsset, clusterName string) InstallerAsset {
	for _, asset := range assets {
		if asset.ClusterName == clusterName {
			return asset
		}
	}
	return InstallerAsset{}
}

func writeYAML(path string, value any) error {
	data, err := yaml.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	if err := atomicWriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tempPath := file.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace file: %w", err)
	}
	cleanup = false
	return nil
}

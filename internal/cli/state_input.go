package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/crmarques/gitups/api/v1alpha1"
	"github.com/crmarques/gitups/internal/infra"
)

const bootstrapRepoName = "clusters-bootstrap.git"

func bootstrapRepoDir(stateDir string) string {
	return filepath.Join(stateDir, bootstrapRepoName)
}

func loadDesiredState(cf *commonFlags) (v1alpha1.State, error) {
	paths, err := desiredStatePaths(cf)
	if err != nil {
		return v1alpha1.State{}, err
	}
	return infra.LoadNormalizeValidate(paths)
}

func loadOptionalDesiredState(cf *commonFlags) (v1alpha1.State, error) {
	paths, err := desiredStatePaths(cf)
	if err != nil {
		if len(cf.files) == 0 && errors.Is(err, os.ErrNotExist) {
			return v1alpha1.State{}, nil
		}
		return v1alpha1.State{}, err
	}
	return infra.LoadNormalizeValidate(paths)
}

func desiredStatePaths(cf *commonFlags) ([]string, error) {
	if len(cf.files) > 0 {
		return cf.files, nil
	}
	paths, err := bootstrapGitupsDirs(cf.stateDir)
	if err != nil {
		return nil, err
	}
	return paths, nil
}

func bootstrapGitupsDirs(stateDir string) ([]string, error) {
	repo := bootstrapRepoDir(stateDir)
	entries, err := os.ReadDir(repo)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s not found; run `gitups init-repo --cluster-name <name> --provider <provider>` or pass -f", err, repo)
		}
		return nil, fmt.Errorf("read %s: %w", repo, err)
	}
	var paths []string
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == ".git" {
			continue
		}
		path := filepath.Join(repo, entry.Name(), "gitups")
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			paths = append(paths, path)
			continue
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("stat %s: %w", path, err)
		}
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, fmt.Errorf("no cluster gitups directories found under %s", repo)
	}
	return paths, nil
}

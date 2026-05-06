package infra

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/crmarques/ztp-ocp-install-lab/api/v1alpha1"
	"go.yaml.in/yaml/v3"
)

func LoadNormalizeValidate(paths []string) (v1alpha1.State, error) {
	state, err := Load(paths)
	if err != nil {
		return v1alpha1.State{}, err
	}
	Normalize(&state)
	if err := Validate(state); err != nil {
		return v1alpha1.State{}, err
	}
	return state, nil
}

func Load(paths []string) (v1alpha1.State, error) {
	files, err := discoverFiles(paths)
	if err != nil {
		return v1alpha1.State{}, err
	}
	var state v1alpha1.State
	for _, file := range files {
		if err := loadFile(file, &state); err != nil {
			return v1alpha1.State{}, err
		}
	}
	if len(state.InfrastructureProviders) == 0 &&
		len(state.Environments) == 0 &&
		len(state.ClusterInfrastructures) == 0 &&
		len(state.OCPClusters) == 0 {
		return v1alpha1.State{}, errors.New("no Gitups YAML documents found")
	}
	sortState(&state)
	return state, nil
}

func discoverFiles(paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, errors.New("at least one -f path is required")
	}
	seen := map[string]bool{}
	var files []string
	for _, input := range paths {
		if strings.TrimSpace(input) == "" {
			return nil, errors.New("empty -f path is not allowed")
		}
		info, err := os.Stat(input)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", input, err)
		}
		if !info.IsDir() {
			if !isYAMLFile(input) {
				return nil, fmt.Errorf("%s is not a .yaml or .yml file", input)
			}
			clean := filepath.Clean(input)
			if !seen[clean] {
				seen[clean] = true
				files = append(files, clean)
			}
			continue
		}
		err = filepath.WalkDir(input, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				base := filepath.Base(path)
				if strings.HasPrefix(base, ".") && filepath.Clean(path) != filepath.Clean(input) {
					return filepath.SkipDir
				}
				return nil
			}
			if !isYAMLFile(path) {
				return nil
			}
			clean := filepath.Clean(path)
			if !seen[clean] {
				seen[clean] = true
				files = append(files, clean)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walk %s: %w", input, err)
		}
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, errors.New("no .yaml or .yml files found")
	}
	return files, nil
}

func loadFile(path string, state *v1alpha1.State) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	for index := 1; ; index++ {
		var node yaml.Node
		err := decoder.Decode(&node)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("decode %s document %d: %w", path, index, err)
		}
		if isZeroNode(node) {
			continue
		}
		var typeMeta v1alpha1.TypeMeta
		if err := node.Decode(&typeMeta); err != nil {
			return fmt.Errorf("decode %s document %d metadata: %w", path, index, err)
		}
		if typeMeta.APIVersion == "" {
			return fmt.Errorf("decode %s document %d: apiVersion is required", path, index)
		}
		if typeMeta.APIVersion != v1alpha1.APIVersion {
			return fmt.Errorf("decode %s document %d: unsupported apiVersion %q", path, index, typeMeta.APIVersion)
		}
		if !mappingHasKey(node, "metadata") {
			return fmt.Errorf("decode %s document %d: metadata is required", path, index)
		}
		if !mappingHasKey(node, "spec") {
			return fmt.Errorf("decode %s document %d: spec is required", path, index)
		}
		switch typeMeta.Kind {
		case v1alpha1.KindEnvironment:
			var item v1alpha1.Environment
			if err := decodeKnown(node, &item); err != nil {
				return fmt.Errorf("decode %s document %d: %w", path, index, err)
			}
			item.SourcePath = path
			state.Environments = append(state.Environments, item)
		case v1alpha1.KindInfrastructureProvider:
			var item v1alpha1.InfrastructureProvider
			if err := decodeKnown(node, &item); err != nil {
				return fmt.Errorf("decode %s document %d: %w", path, index, err)
			}
			item.SourcePath = path
			state.InfrastructureProviders = append(state.InfrastructureProviders, item)
		case v1alpha1.KindClusterInfrastructure:
			var item v1alpha1.ClusterInfrastructure
			if err := decodeKnown(node, &item); err != nil {
				return fmt.Errorf("decode %s document %d: %w", path, index, err)
			}
			item.SourcePath = path
			state.ClusterInfrastructures = append(state.ClusterInfrastructures, item)
		case v1alpha1.KindOCPCluster:
			var item v1alpha1.OCPCluster
			if err := decodeKnown(node, &item); err != nil {
				return fmt.Errorf("decode %s document %d: %w", path, index, err)
			}
			item.SourcePath = path
			state.OCPClusters = append(state.OCPClusters, item)
		case "":
			return fmt.Errorf("decode %s document %d: kind is required", path, index)
		default:
			return fmt.Errorf("decode %s document %d: unsupported kind %q", path, index, typeMeta.Kind)
		}
	}
	return nil
}

func mappingHasKey(node yaml.Node, key string) bool {
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		node = *node.Content[0]
	}
	if node.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return true
		}
	}
	return false
}

func decodeKnown(node yaml.Node, value any) error {
	data, err := yaml.Marshal(&node)
	if err != nil {
		return err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	return decoder.Decode(value)
}

func sortState(state *v1alpha1.State) {
	sort.Slice(state.Environments, func(i, j int) bool {
		if state.Environments[i].Metadata.Name == state.Environments[j].Metadata.Name {
			return state.Environments[i].SourcePath < state.Environments[j].SourcePath
		}
		return state.Environments[i].Metadata.Name < state.Environments[j].Metadata.Name
	})
	sort.Slice(state.InfrastructureProviders, func(i, j int) bool {
		if state.InfrastructureProviders[i].Metadata.Name == state.InfrastructureProviders[j].Metadata.Name {
			return state.InfrastructureProviders[i].SourcePath < state.InfrastructureProviders[j].SourcePath
		}
		return state.InfrastructureProviders[i].Metadata.Name < state.InfrastructureProviders[j].Metadata.Name
	})
	sort.Slice(state.ClusterInfrastructures, func(i, j int) bool {
		if state.ClusterInfrastructures[i].Metadata.Name == state.ClusterInfrastructures[j].Metadata.Name {
			return state.ClusterInfrastructures[i].SourcePath < state.ClusterInfrastructures[j].SourcePath
		}
		return state.ClusterInfrastructures[i].Metadata.Name < state.ClusterInfrastructures[j].Metadata.Name
	})
	sort.Slice(state.OCPClusters, func(i, j int) bool {
		if state.OCPClusters[i].Metadata.Name == state.OCPClusters[j].Metadata.Name {
			return state.OCPClusters[i].SourcePath < state.OCPClusters[j].SourcePath
		}
		return state.OCPClusters[i].Metadata.Name < state.OCPClusters[j].Metadata.Name
	})
}

func isYAMLFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}

func isZeroNode(node yaml.Node) bool {
	if node.Kind == 0 {
		return true
	}
	if node.Kind == yaml.DocumentNode && len(node.Content) == 0 {
		return true
	}
	if node.Kind == yaml.DocumentNode && len(node.Content) == 1 {
		child := node.Content[0]
		return child.Kind == yaml.ScalarNode && child.Tag == "!!null"
	}
	return false
}

package safepath

import (
	"fmt"
	"path/filepath"
	"strings"
)

func Relative(label, value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s is required", label)
	}
	if filepath.IsAbs(value) {
		return "", fmt.Errorf("%s %q must be relative", label, value)
	}
	clean := filepath.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s %q must stay within the workspace", label, value)
	}
	return clean, nil
}

func Name(label, value string) error {
	clean, err := Relative(label, value)
	if err != nil {
		return err
	}
	if clean != value || strings.ContainsAny(value, `/\`) {
		return fmt.Errorf("%s %q must be a single path segment", label, value)
	}
	return nil
}

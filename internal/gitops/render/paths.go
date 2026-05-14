package render

import (
	"path/filepath"

	"github.com/crmarques/bootwright/internal/gitops/safepath"
)

func sourcePath(sourceDir, label, value string) (string, error) {
	clean, err := safepath.Relative(label, value)
	if err != nil {
		return "", err
	}
	return filepath.Join(sourceDir, clean), nil
}

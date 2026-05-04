package cli

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	gitupsHomeEnv       = "GITUPS_HOME"
	defaultHostStateDir = "/var/lib/gitups"
)

func defaultGitupsHome() string {
	if value := strings.TrimSpace(os.Getenv(gitupsHomeEnv)); value != "" {
		return filepath.Clean(value)
	}
	home, err := os.UserHomeDir()
	if err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".gitups")
	}
	return ".gitups"
}

func defaultStateDir() string {
	return filepath.Join(defaultGitupsHome(), "state")
}

func defaultSecretsDir() string {
	return filepath.Join(defaultGitupsHome(), "secrets")
}

func openshiftInstallSearchDirs(hostStateDir string) []string {
	return []string{"/usr/local/bin", filepath.Join(hostStateDir, "tools")}
}

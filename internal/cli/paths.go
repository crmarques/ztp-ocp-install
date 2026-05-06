package cli

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	gitupsHomeEnv       = "GITUPS_HOME"
	defaultHostStateDir = "/var/lib/gitups"
	ansibleVenvDirName  = "ansible-venv"
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
	return []string{defaultControllerCLIInstallDir(), "/usr/local/bin", filepath.Join(hostStateDir, "tools")}
}

func defaultControllerCLIInstallDir() string {
	return filepath.Join(defaultGitupsHome(), "bin")
}

func ansibleVenvDir() string {
	return filepath.Join(defaultGitupsHome(), ansibleVenvDirName)
}

func ansibleVenvBin(name string) string {
	return filepath.Join(ansibleVenvDir(), "bin", name)
}

func resolveAnsiblePlaybook() string {
	bin := ansibleVenvBin("ansible-playbook")
	if isExecutable(bin) {
		return bin
	}
	return "ansible-playbook"
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}

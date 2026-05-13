package cli

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	gitupsUserDirEnv    = "GITUPS_USER_DIR"
	gitupsStateDirEnv   = "GITUPS_STATE_DIR"
	gitupsSecretsDirEnv = "GITUPS_SECRETS_DIR"

	defaultHostStateDir = "/var/lib/gitups"
	ansibleVenvDirName  = "ansible-venv"
)

func defaultGitupsUserDir() string {
	if value := strings.TrimSpace(os.Getenv(gitupsUserDirEnv)); value != "" {
		return filepath.Clean(value)
	}
	home, err := os.UserHomeDir()
	if err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".gitups")
	}
	return ".gitups"
}

func defaultStateDir() string {
	if value := strings.TrimSpace(os.Getenv(gitupsStateDirEnv)); value != "" {
		return filepath.Clean(value)
	}
	if found, ok := discoverStateDir(); ok {
		return found
	}
	return filepath.Join(defaultGitupsUserDir(), "state")
}

func discoverStateDir() (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for i := 0; i < 32; i++ {
		info, err := os.Stat(filepath.Join(dir, bootstrapRepoName))
		if err == nil && info.IsDir() {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
	return "", false
}

func defaultSecretsDir() string {
	if value := strings.TrimSpace(os.Getenv(gitupsSecretsDirEnv)); value != "" {
		return filepath.Clean(value)
	}
	return filepath.Join(defaultGitupsUserDir(), "secrets")
}

func openshiftInstallSearchDirs(_ string) []string {
	return []string{defaultControllerCLIInstallDir()}
}

func defaultControllerCLIInstallDir() string {
	return "/usr/local/bin"
}

func ansibleVenvDir() string {
	return filepath.Join(defaultGitupsUserDir(), ansibleVenvDirName)
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

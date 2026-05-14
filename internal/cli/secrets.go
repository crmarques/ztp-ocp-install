package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/crmarques/bootwright/api/v1alpha1"
	"github.com/crmarques/bootwright/internal/infra"
	"github.com/crmarques/bootwright/internal/secretref"
)

type generatedSelfSignedRequest struct {
	name        string
	certificate v1alpha1.SelfSignedCertificateSpec
}

func newSecretCmd(stdout io.Writer, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secret",
		Short: "Manage local install secret material",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(
		newSecretSetCmd(stdout),
		newSecretGenerateCmd(stdout, stderr),
	)
	showSubcommandFlagsInHelp(cmd)
	return cmd
}

func resolvedSecretPath(name string, env *v1alpha1.Environment, secretsDir string) string {
	return secretref.ResolvePath(name, env, secretsDir)
}

func primaryEnvironmentForSync(state v1alpha1.State) *v1alpha1.Environment {
	if len(state.Environments) == 0 {
		return nil
	}
	return &state.Environments[0]
}

func newSecretGenerateCmd(stdout io.Writer, _ io.Writer) *cobra.Command {
	var (
		files      []string
		secretsDir string
	)
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate local install secret material requested by desired state",
		Args:  cobra.NoArgs,
	}
	secretsDir = defaultSecretsDir()
	cmd.Flags().StringArrayVarP(&files, "file", "f", nil, "Bootwright YAML file or directory; may be repeated")
	cmd.Flags().StringVar(&secretsDir, "secrets-dir", secretsDir, "directory for generated install secret material")
	cmd.RunE = func(_ *cobra.Command, _ []string) error {
		state, err := infra.LoadNormalizeValidate(files)
		if err != nil {
			return failErr(1, err)
		}
		certRequests, err := generatedSelfSignedRequests(state)
		if err != nil {
			return failErr(1, err)
		}
		credRequests := generatedCredentialsRequestsFor(state)
		printTitle(stdout, "secret generate")
		if len(certRequests) == 0 && len(credRequests) == 0 {
			fmt.Fprintln(stdout, "secret generate: no generated secret requests found")
			return nil
		}
		if err := os.MkdirAll(secretsDir, 0o700); err != nil {
			return failErr(1, fmt.Errorf("create secrets directory %s: %w", secretsDir, err))
		}
		if err := os.Chmod(secretsDir, 0o700); err != nil {
			return failErr(1, fmt.Errorf("chmod secrets directory %s: %w", secretsDir, err))
		}
		for _, request := range certRequests {
			action, err := materializeSelfSignedCertificate(secretsDir, request)
			if err != nil {
				return failErr(1, err)
			}
			printOK(stdout, request.name, action)
		}
		for _, request := range credRequests {
			action, err := materializeGeneratedCredentials(secretsDir, request)
			if err != nil {
				return failErr(1, err)
			}
			printOK(stdout, request.name, action)
		}
		return nil
	}
	return cmd
}

func materializeGeneratedCredentials(secretsDir string, request generatedCredentialsRequest) (string, error) {
	target := filepath.Join(secretsDir, request.name)
	wantUser := request.credentials.Username
	if wantUser == "" {
		wantUser = "admin"
	}
	exists, err := regularFileExists(target)
	if err != nil {
		return "", err
	}
	if exists {
		data, err := os.ReadFile(target)
		if err != nil {
			return "", fmt.Errorf("read existing credentials %s: %w", target, err)
		}
		gotUser, _, perr := parseBMCCredentials(data)
		if perr != nil {
			return "", fmt.Errorf("existing credentials %s: %w; remove the file to regenerate", target, perr)
		}
		if gotUser != wantUser {
			return "", fmt.Errorf("existing credentials %q at %s use username %q but desired spec wants %q; remove %s to regenerate", request.name, target, gotUser, wantUser, target)
		}
		return "reused existing credentials", nil
	}
	password, err := generateBMCPassword()
	if err != nil {
		return "", err
	}
	payload := []byte(wantUser + ":" + password + "\n")
	if err := atomicWriteFile(target, payload, 0o600); err != nil {
		return "", err
	}
	return fmt.Sprintf("generated %s (user %q)", target, wantUser), nil
}

func newSecretSetCmd(stdout io.Writer) *cobra.Command {
	var (
		pullSecret    string
		fromFile      string
		username      string
		password      string
		passwordStdin bool
		generate      bool
		secretsDir    string
	)
	secretsDir = defaultSecretsDir()
	cmd := &cobra.Command{
		Use:   "set <name>",
		Short: "Store a pull secret or username:password credentials for a SecretRef",
		Long: `Write the named SecretRef material to <secrets-dir>/<name>, mode 0600.
Exactly one input mode is required:

  --pull-secret <file>             store an OpenShift pull-secret JSON file
  --from-file <file>               store an existing "username:password" file
  --username <u> --password <p>    store inline credentials
  --username <u> --password-stdin  read the password from stdin
  --generate                       generate a random password (default username "admin")

Use --generate for test fixtures; use the other modes for material the
operator provides.`,
		Args: cobra.ExactArgs(1),
	}
	cmd.Flags().StringVar(&pullSecret, "pull-secret", "", "path to an OpenShift pull-secret JSON file")
	cmd.Flags().StringVar(&fromFile, "from-file", "", "path to a file containing one line: username:password")
	cmd.Flags().StringVar(&username, "username", "", "username (required with --password, --password-stdin, or --generate)")
	cmd.Flags().StringVar(&password, "password", "", "password (mutually exclusive with --password-stdin and --generate)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "read password from stdin instead of --password")
	cmd.Flags().BoolVar(&generate, "generate", false, "generate a strong random password (intended for test fixtures)")
	cmd.Flags().StringVar(&secretsDir, "secrets-dir", secretsDir, "directory for local install secret material (env: BOOTWRIGHT_SECRETS_DIR)")
	cmd.RunE = func(c *cobra.Command, args []string) error {
		name := args[0]
		if !infra.IsDNSLabel(name) {
			return failf(2, "<name> must be a lowercase DNS label")
		}
		modes := 0
		if pullSecret != "" {
			modes++
		}
		if fromFile != "" {
			modes++
		}
		if password != "" {
			modes++
		}
		if passwordStdin {
			modes++
		}
		if generate {
			modes++
		}
		if modes == 0 {
			return failf(2, "one of --pull-secret, --from-file, --password, --password-stdin, or --generate is required")
		}
		if modes > 1 {
			return failf(2, "--pull-secret, --from-file, --password, --password-stdin, and --generate are mutually exclusive")
		}
		if pullSecret != "" {
			return runSecretSetPullSecret(stdout, name, pullSecret, secretsDir)
		}
		return runSecretSetCredentials(c, stdout, name, fromFile, username, password, passwordStdin, generate, secretsDir)
	}
	return cmd
}

func runSecretSetPullSecret(stdout io.Writer, name, fromFile, secretsDir string) error {
	data, err := os.ReadFile(fromFile)
	if err != nil {
		return failErr(1, fmt.Errorf("read pull secret file %s: %w", fromFile, err))
	}
	if err := validatePullSecretJSON(data); err != nil {
		return failErr(1, err)
	}
	if err := os.MkdirAll(secretsDir, 0o700); err != nil {
		return failErr(1, fmt.Errorf("create secrets directory %s: %w", secretsDir, err))
	}
	if err := os.Chmod(secretsDir, 0o700); err != nil {
		return failErr(1, fmt.Errorf("chmod secrets directory %s: %w", secretsDir, err))
	}
	target := filepath.Join(secretsDir, name)
	exists, err := regularFileExists(target)
	if err != nil {
		return failErr(1, err)
	}
	if err := atomicWriteFile(target, data, 0o600); err != nil {
		return failErr(1, err)
	}
	action := "wrote"
	if exists {
		action = "updated"
	}
	printTitle(stdout, "secret set")
	printOK(stdout, name, fmt.Sprintf("%s pull secret at %s", action, target))
	return nil
}

func runSecretSetCredentials(c *cobra.Command, stdout io.Writer, name, fromFile, username, password string, passwordStdin, generate bool, secretsDir string) error {
	var resolvedUser, resolvedPass string
	switch {
	case fromFile != "":
		data, err := os.ReadFile(fromFile)
		if err != nil {
			return failErr(1, fmt.Errorf("read credentials file %s: %w", fromFile, err))
		}
		u, p, err := parseBMCCredentials(data)
		if err != nil {
			return failErr(1, err)
		}
		resolvedUser, resolvedPass = u, p
	case password != "":
		if username == "" {
			return failf(2, "--username is required with --password")
		}
		resolvedUser, resolvedPass = username, password
	case passwordStdin:
		if username == "" {
			return failf(2, "--username is required with --password-stdin")
		}
		stdin := c.InOrStdin()
		if stdin == nil {
			return failErr(1, errors.New("--password-stdin requires stdin"))
		}
		line, err := bufio.NewReader(stdin).ReadString('\n')
		if err != nil && line == "" {
			return failErr(1, fmt.Errorf("read password from stdin: %w", err))
		}
		resolvedUser, resolvedPass = username, strings.TrimRight(line, "\r\n")
	case generate:
		resolvedUser = username
		if resolvedUser == "" {
			resolvedUser = "admin"
		}
		generated, err := generateBMCPassword()
		if err != nil {
			return failErr(1, err)
		}
		resolvedPass = generated
	}
	if err := validateBMCUsername(resolvedUser); err != nil {
		return failErr(1, err)
	}
	if resolvedPass == "" {
		return failErr(1, errors.New("password must not be empty"))
	}
	if err := os.MkdirAll(secretsDir, 0o700); err != nil {
		return failErr(1, fmt.Errorf("create secrets directory %s: %w", secretsDir, err))
	}
	if err := os.Chmod(secretsDir, 0o700); err != nil {
		return failErr(1, fmt.Errorf("chmod secrets directory %s: %w", secretsDir, err))
	}
	target := filepath.Join(secretsDir, name)
	exists, err := regularFileExists(target)
	if err != nil {
		return failErr(1, err)
	}
	payload := []byte(resolvedUser + ":" + resolvedPass + "\n")
	if err := atomicWriteFile(target, payload, 0o600); err != nil {
		return failErr(1, err)
	}
	action := "wrote"
	if exists {
		action = "updated"
	}
	printTitle(stdout, "secret set")
	message := fmt.Sprintf("%s credentials at %s (user %q)", action, target, resolvedUser)
	if generate {
		message += " — password generated; copy it from the file above before sharing"
	}
	printOK(stdout, name, message)
	return nil
}

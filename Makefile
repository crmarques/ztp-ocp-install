GO ?= go
BINARY ?= gitups
BIN_DIR ?= bin
STATE_DIR ?= .state
E2E_DIR ?= test/e2e
CASE ?=
E2E_FIXTURE = $(E2E_DIR)/$(CASE)
E2E_STATE_DIR ?= /tmp/gitups-$(CASE)
ANSIBLE_PLAYBOOK ?= $(shell command -v ansible-playbook 2>/dev/null)
E2E_ANSIBLE_FLAGS = $(if $(ANSIBLE_PLAYBOOK),--ansible-playbook $(ANSIBLE_PLAYBOOK),)
E2E_APPLY_ALL ?= $(BIN_DIR)/$(BINARY) apply all --yes
E2E_APPLY_FLAGS ?=
E2E_CLEAN ?= sudo rm -rf

ANSIBLE_SRC_DIR = ansible
EMBED_BUNDLE_DIR = internal/embedded/bundle
ANSIBLE_GALAXY ?= $(shell command -v ansible-galaxy 2>/dev/null)
COLLECTIONS_REQUIREMENTS = $(ANSIBLE_SRC_DIR)/collections/requirements.yml
EMBED_COLLECTIONS_DIR = $(EMBED_BUNDLE_DIR)/collections
COLLECTIONS_STAMP = $(EMBED_COLLECTIONS_DIR)/.stamp
GOFMT_FILES = $(shell find . -path './internal/embedded/bundle' -prune -o -name '*.go' -print)
ANSIBLE_ROLE_PATHS = ansible/roles/bastion:ansible/roles/shared:ansible/roles/providers:ansible/roles/cluster_infra:ansible/roles/openshift
ANSIBLE_SYNTAX_ENV = ANSIBLE_LOCAL_TEMP=/tmp/gitups-ansible-local ANSIBLE_REMOTE_TEMP=/tmp/gitups-ansible-remote ANSIBLE_ROLES_PATH=$(ANSIBLE_ROLE_PATHS) ANSIBLE_COLLECTIONS_PATH=internal/embedded/bundle/collections ANSIBLE_FILTER_PLUGINS=ansible/filter_plugins
ANSIBLE_SYNTAX_PLAYBOOKS = \
	ansible/playbooks/checks/preflight.yml \
	ansible/playbooks/targets/all/apply.yml \
	ansible/playbooks/targets/infra/apply.yml \
	ansible/playbooks/targets/infra/destroy.yml \
	ansible/playbooks/targets/clusters/apply.yml \
	ansible/playbooks/targets/clusters/destroy.yml \
	ansible/playbooks/targets/bastion/apply-clis.yml

E2E_CASES = $(filter-out gitops,$(notdir $(patsubst %/,%,$(wildcard $(E2E_DIR)/*/))))

GITOPS_E2E_DIR ?= $(E2E_DIR)/gitops
GITOPS_E2E_CASES = $(notdir $(patsubst %/,%,$(wildcard $(GITOPS_E2E_DIR)/*/)))

.PHONY: all build sync-bundle test validate plan check check-gofmt ansible-syntax-check stale-term-check provider-swap-check cli-file-size-check check-e2e-deps check-e2e-case list-e2e-cases e2e-dry-run e2e e2e-render-baremetal e2e-gitops list-e2e-gitops-cases clean clean-e2e-state help

# Architecture guardrail: keep internal/cli files thin so domain logic stays
# in internal/workflow/ and internal/gitops/workflow/. The current observed
# max (init.go ~391) is the floor; do not raise this without a deliberate
# refactor justification. The intent is to catch new growth before files
# turn into god files again.
CLI_FILE_LINE_LIMIT ?= 400

all: build

build: $(BIN_DIR) sync-bundle
	$(GO) build -o $(BIN_DIR)/$(BINARY) ./cmd/gitups

# PLACEHOLDER and .gitignore are preserved so a bare `go build` still
# compiles when the bundle has not been synced. Collections declared in
# $(COLLECTIONS_REQUIREMENTS) are resolved at build time into
# $(EMBED_COLLECTIONS_DIR) so the gitups binary ships every Ansible
# collection it needs and disconnected hosts never reach Galaxy.
sync-bundle: $(COLLECTIONS_STAMP)
	@find $(EMBED_BUNDLE_DIR) -mindepth 1 -maxdepth 1 \
		! -name PLACEHOLDER ! -name .gitignore ! -name collections -exec rm -rf {} +
	@cp -R $(ANSIBLE_SRC_DIR)/. $(EMBED_BUNDLE_DIR)/

# Galaxy download is gated on requirements.yml — only re-runs when it changes.
$(COLLECTIONS_STAMP): $(COLLECTIONS_REQUIREMENTS)
	@test -n "$(ANSIBLE_GALAXY)" || { printf '%s\n' 'ansible-galaxy not found in PATH; install Ansible or set ANSIBLE_GALAXY=/path/to/ansible-galaxy'; exit 1; }
	@rm -rf $(EMBED_COLLECTIONS_DIR)/ansible_collections
	@$(ANSIBLE_GALAXY) collection install -r $(COLLECTIONS_REQUIREMENTS) -p $(EMBED_COLLECTIONS_DIR) >/dev/null
	@# Slim embedded collections: strip test/CI/docs trees that bloat the
	@# binary without contributing to runtime module execution.
	@find $(EMBED_COLLECTIONS_DIR)/ansible_collections -maxdepth 8 -type d \( \
		-name tests -o -name docs -o -name examples -o -name changelogs \
		-o -name .github -o -name .azure-pipelines -o -name ci \
		\) -exec rm -rf {} +
	@touch $@

$(BIN_DIR):
	mkdir -p $(BIN_DIR)

test:
	$(GO) test ./...

check: check-gofmt
	$(GO) vet ./...
	$(GO) test ./...
	$(MAKE) ansible-syntax-check
	$(MAKE) stale-term-check
	$(MAKE) provider-swap-check
	$(MAKE) cli-file-size-check

check-gofmt:
	@test -z "$$(gofmt -l $(GOFMT_FILES))" || { gofmt -l $(GOFMT_FILES); exit 1; }

ansible-syntax-check: check-e2e-deps
	@for playbook in $(ANSIBLE_SYNTAX_PLAYBOOKS); do \
		$(ANSIBLE_SYNTAX_ENV) $(ANSIBLE_PLAYBOOK) --syntax-check -i localhost, "$$playbook"; \
	done

stale-term-check:
	@! rg -n 'connectivity[.](mode|connected|restricted|disconnected)|spec[.]connectivity|gitups_connectivity|localRegistry|ocpInstall[.]|spoke|examples/infra' README.md docs specs examples test

provider-swap-check:
	diff -u examples/libvirt-redfish-fleet/environment.yaml examples/baremetal-redfish-fleet/environment.yaml
	diff -u examples/libvirt-redfish-fleet/ocp-cluster-hub.yaml examples/baremetal-redfish-fleet/ocp-cluster-hub.yaml
	diff -u examples/libvirt-redfish-fleet/ocp-cluster-managed-01.yaml examples/baremetal-redfish-fleet/ocp-cluster-managed-01.yaml

# Reject CLI files that have grown past the thin-handler threshold. Excludes
# test files so the lint targets production code only.
cli-file-size-check:
	@over=$$(find internal/cli -maxdepth 1 -type f -name '*.go' ! -name '*_test.go' -printf '%p\n' \
		| while read -r f; do \
			n=$$(wc -l <"$$f"); \
			if [ "$$n" -gt $(CLI_FILE_LINE_LIMIT) ]; then printf '  %s lines\t%s\n' "$$n" "$$f"; fi; \
		done); \
	if [ -n "$$over" ]; then \
		printf '%s\n' "internal/cli files over $(CLI_FILE_LINE_LIMIT) lines (move domain logic into internal/workflow/ or internal/gitops/workflow/):" "$$over"; \
		exit 1; \
	fi

validate: build
	$(BIN_DIR)/$(BINARY) check all -f examples/libvirt-redfish-fleet --state-dir $(STATE_DIR) --dry-run

plan: build
	$(BIN_DIR)/$(BINARY) apply all -f examples/libvirt-redfish-fleet --state-dir $(STATE_DIR) --dry-run

check-e2e-deps:
	@test -n "$(ANSIBLE_PLAYBOOK)" || { printf '%s\n' 'ansible-playbook not found in PATH; install Ansible or set ANSIBLE_PLAYBOOK=/path/to/ansible-playbook'; exit 1; }

check-e2e-case:
	@test -n "$(CASE)" || { printf '%s\n' 'CASE is required; pass CASE=<name>, e.g. make e2e CASE=local-libvirt-sno-hub-gitea' 'Available cases:' $(addprefix '  ',$(E2E_CASES)); exit 1; }
	@test -d "$(E2E_FIXTURE)" || { printf '%s\n' 'CASE "$(CASE)" not found at $(E2E_FIXTURE)' 'Available cases:' $(addprefix '  ',$(E2E_CASES)); exit 1; }

list-e2e-cases:
	@printf '%s\n' 'Available e2e cases:' $(addprefix '  ',$(E2E_CASES))

e2e-dry-run: check-e2e-case check-e2e-deps build
	$(BIN_DIR)/$(BINARY) apply all -f $(E2E_FIXTURE) --state-dir $(E2E_STATE_DIR) --dry-run $(E2E_ANSIBLE_FLAGS) $(E2E_APPLY_FLAGS)

e2e: check-e2e-case check-e2e-deps build
	$(E2E_APPLY_ALL) -f $(E2E_FIXTURE) --state-dir $(E2E_STATE_DIR) $(E2E_ANSIBLE_FLAGS) $(E2E_APPLY_FLAGS)

# Render-only e2e for the baremetal-redfish fleet — exercises the
# bare-metal provider dispatch path without requiring real Redfish
# hardware. Produces inventory/vars/installer artifacts under
# /tmp/gitups-baremetal-redfish-fleet and prints the Ansible command,
# but never invokes ansible-playbook.
e2e-render-baremetal: check-e2e-deps build
	$(BIN_DIR)/$(BINARY) apply all -f examples/baremetal-redfish-fleet \
		--state-dir /tmp/gitups-baremetal-redfish-fleet --dry-run \
		$(E2E_ANSIBLE_FLAGS) $(E2E_APPLY_FLAGS)

list-e2e-gitops-cases:
	@printf '%s\n' 'Available gitops e2e cases:' $(addprefix '  ',$(GITOPS_E2E_CASES))

# e2e-gitops runs `check gitops`, `expand gitops`, and `render gitops`
# against a fixture under test/e2e/gitops/<case>/. It is read-only against
# the cluster (no apply/push) — the goal is to validate the merged tool's
# gitops pipeline without touching a remote git provider or a kind cluster.
e2e-gitops: build
	@test -n "$(CASE)" || { printf '%s\n' 'CASE is required; pass CASE=<name>, e.g. make e2e-gitops CASE=test1' 'Available cases:' $(addprefix '  ',$(GITOPS_E2E_CASES)); exit 1; }
	$(BIN_DIR)/$(BINARY) check gitops $(CASE) -d $(GITOPS_E2E_DIR)
	$(BIN_DIR)/$(BINARY) expand gitops $(CASE) -d $(GITOPS_E2E_DIR) --force
	$(BIN_DIR)/$(BINARY) render gitops $(CASE) -d $(GITOPS_E2E_DIR) --allow-placeholders

clean:
	rm -rf $(BIN_DIR) $(STATE_DIR) dist build out rendered tmp
	@find $(EMBED_BUNDLE_DIR) -mindepth 1 -maxdepth 1 \
		! -name PLACEHOLDER ! -name .gitignore -exec rm -rf {} +

clean-e2e-state: check-e2e-case
	$(E2E_CLEAN) $(E2E_STATE_DIR)

help:
	@printf '%s\n' \
		'Targets:' \
		'  build            Build bin/gitups (syncs the embedded ansible bundle first)' \
		'  sync-bundle      Refresh internal/embedded/bundle from /ansible without building' \
		'  check            Run formatting, Go, Ansible, stale-term, and provider-swap checks' \
		'  test             Run Go tests' \
		'  validate         Validate examples/libvirt-redfish-fleet' \
		'  plan             Preview examples/libvirt-redfish-fleet into .state' \
		'  list-e2e-cases   List available e2e cases under test/e2e' \
		'  check-e2e-deps   Check local e2e dependencies' \
		'  e2e-dry-run         Render an e2e fixture and print Ansible command (requires CASE=<name>)' \
		'  e2e                 Run an e2e fixture with sudo (requires CASE=<name>)' \
		'  e2e-render-baremetal  Render-only e2e for examples/baremetal-redfish-fleet (no hardware needed)' \
		'  list-e2e-gitops-cases  List available gitops e2e cases under test/e2e/gitops' \
		'  e2e-gitops          Run gitops check/expand/render against a fixture (requires CASE=<name>)' \
		'  clean               Remove workspace-local generated outputs' \
		'  clean-e2e-state     Remove generated e2e CLI state with sudo (requires CASE=<name>)'

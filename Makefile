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
E2E_APPLY_INFRA ?= $(BIN_DIR)/$(BINARY) apply infra --yes
E2E_APPLY_OCP ?= $(BIN_DIR)/$(BINARY) apply ocp --yes
E2E_APPLY_FLAGS ?=
E2E_DESTROY ?= $(BIN_DIR)/$(BINARY) destroy all
E2E_DESTROY_FLAGS ?=
E2E_CLEAN ?= sudo rm -rf

ANSIBLE_SRC_DIR = ansible
EMBED_BUNDLE_DIR = internal/embedded/bundle
ANSIBLE_GALAXY ?= $(shell command -v ansible-galaxy 2>/dev/null)
COLLECTIONS_REQUIREMENTS = $(ANSIBLE_SRC_DIR)/collections/requirements.yml
EMBED_COLLECTIONS_DIR = $(EMBED_BUNDLE_DIR)/collections

E2E_CASES = $(notdir $(patsubst %/,%,$(wildcard $(E2E_DIR)/*/)))

.PHONY: all build sync-bundle test validate plan check-e2e-deps check-e2e-case list-e2e-cases e2e-dry-run e2e e2e-destroy-dry-run e2e-destroy clean clean-e2e-state help

all: build

build: $(BIN_DIR) sync-bundle
	$(GO) build -o $(BIN_DIR)/$(BINARY) ./cmd/gitups

# PLACEHOLDER and .gitignore are preserved so a bare `go build` still
# compiles when the bundle has not been synced. Collections declared in
# $(COLLECTIONS_REQUIREMENTS) are resolved at build time into
# $(EMBED_COLLECTIONS_DIR) so the gitups binary ships every Ansible
# collection it needs and disconnected hosts never reach Galaxy.
sync-bundle:
	@find $(EMBED_BUNDLE_DIR) -mindepth 1 -maxdepth 1 \
		! -name PLACEHOLDER ! -name .gitignore -exec rm -rf {} +
	@cp -R $(ANSIBLE_SRC_DIR)/. $(EMBED_BUNDLE_DIR)/
	@test -n "$(ANSIBLE_GALAXY)" || { printf '%s\n' 'ansible-galaxy not found in PATH; install Ansible or set ANSIBLE_GALAXY=/path/to/ansible-galaxy'; exit 1; }
	@$(ANSIBLE_GALAXY) collection install --force -r $(COLLECTIONS_REQUIREMENTS) -p $(EMBED_COLLECTIONS_DIR) >/dev/null
	@# Slim embedded collections: strip test/CI/docs trees that bloat the
	@# binary without contributing to runtime module execution.
	@find $(EMBED_COLLECTIONS_DIR)/ansible_collections -maxdepth 4 -type d \( \
		-name tests -o -name docs -o -name changelogs \
		-o -name .github -o -name .azure-pipelines -o -name ci \
		\) -exec rm -rf {} +

$(BIN_DIR):
	mkdir -p $(BIN_DIR)

test:
	$(GO) test ./...

validate: build
	$(BIN_DIR)/$(BINARY) validate -f examples/libvirt-redfish-fleet

plan: build
	$(BIN_DIR)/$(BINARY) plan -f examples/libvirt-redfish-fleet --state-dir $(STATE_DIR)

check-e2e-deps:
	@test -n "$(ANSIBLE_PLAYBOOK)" || { printf '%s\n' 'ansible-playbook not found in PATH; install Ansible or set ANSIBLE_PLAYBOOK=/path/to/ansible-playbook'; exit 1; }

check-e2e-case:
	@test -n "$(CASE)" || { printf '%s\n' 'CASE is required; pass CASE=<name>, e.g. make e2e CASE=libvirt-1-host-1-sno-hub' 'Available cases:' $(addprefix '  ',$(E2E_CASES)); exit 1; }
	@test -d "$(E2E_FIXTURE)" || { printf '%s\n' 'CASE "$(CASE)" not found at $(E2E_FIXTURE)' 'Available cases:' $(addprefix '  ',$(E2E_CASES)); exit 1; }

list-e2e-cases:
	@printf '%s\n' 'Available e2e cases:' $(addprefix '  ',$(E2E_CASES))

e2e-dry-run: check-e2e-case check-e2e-deps build
	$(BIN_DIR)/$(BINARY) apply infra -f $(E2E_FIXTURE) --state-dir $(E2E_STATE_DIR) --dry-run $(E2E_ANSIBLE_FLAGS) $(E2E_APPLY_FLAGS)
	$(BIN_DIR)/$(BINARY) apply ocp -f $(E2E_FIXTURE) --state-dir $(E2E_STATE_DIR) --dry-run $(E2E_ANSIBLE_FLAGS) $(E2E_APPLY_FLAGS)

e2e: check-e2e-case check-e2e-deps build
	$(E2E_APPLY_INFRA) -f $(E2E_FIXTURE) --state-dir $(E2E_STATE_DIR) $(E2E_ANSIBLE_FLAGS) $(E2E_APPLY_FLAGS)
	$(E2E_APPLY_OCP) -f $(E2E_FIXTURE) --state-dir $(E2E_STATE_DIR) $(E2E_ANSIBLE_FLAGS) $(E2E_APPLY_FLAGS)

e2e-destroy-dry-run: check-e2e-case check-e2e-deps build
	$(BIN_DIR)/$(BINARY) destroy all -f $(E2E_FIXTURE) --state-dir $(E2E_STATE_DIR) --dry-run $(E2E_ANSIBLE_FLAGS) $(E2E_DESTROY_FLAGS)

e2e-destroy: check-e2e-case check-e2e-deps build
	$(E2E_DESTROY) -f $(E2E_FIXTURE) --state-dir $(E2E_STATE_DIR) --yes $(E2E_ANSIBLE_FLAGS) $(E2E_DESTROY_FLAGS)
	$(E2E_CLEAN) $(E2E_STATE_DIR)

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
		'  test             Run Go tests' \
		'  validate         Validate examples/libvirt-redfish-fleet' \
		'  plan             Preview examples/libvirt-redfish-fleet into .state' \
		'  list-e2e-cases   List available e2e cases under test/e2e' \
		'  check-e2e-deps   Check local e2e dependencies' \
		'  e2e-dry-run         Render an e2e fixture and print Ansible command (requires CASE=<name>)' \
		'  e2e                 Run an e2e fixture with sudo (requires CASE=<name>)' \
		'  e2e-destroy-dry-run Render the destroy plan and print Ansible command (requires CASE=<name>)' \
		'  e2e-destroy         Tear down an e2e fixture and remove local state with sudo (requires CASE=<name>)' \
		'  clean               Remove workspace-local generated outputs' \
		'  clean-e2e-state     Remove generated e2e CLI state with sudo (requires CASE=<name>)'

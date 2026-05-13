package workflow

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	v1 "github.com/crmarques/gitups/api/v1alpha1"
)

func TestNewWorkspaceValidatesName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{"empty", "", "required"},
		{"slash", "a/b", "path separators"},
		{"backslash", `a\b`, "path separators"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewWorkspace("/tmp", tc.input)
			if err == nil {
				t.Fatalf("NewWorkspace(%q): expected error", tc.input)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error must mention %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}

func TestNewWorkspaceDefaultsAndPaths(t *testing.T) {
	ws, err := NewWorkspace("", "dev")
	if err != nil {
		t.Fatalf("NewWorkspace: %v", err)
	}
	if ws.Name != "dev" {
		t.Fatalf("Name = %q", ws.Name)
	}
	if !strings.HasSuffix(ws.Root, filepath.Join(DefaultWorkspaceRoot, "dev")) {
		t.Fatalf("Root = %q; expected to end with %q", ws.Root, filepath.Join(DefaultWorkspaceRoot, "dev"))
	}
	if !strings.HasSuffix(ws.PackageSet, filepath.Join("dev", "gitops-package-set.yaml")) {
		t.Fatalf("PackageSet = %q", ws.PackageSet)
	}
	if !strings.HasSuffix(ws.ExpandedPackageSet, filepath.Join(".gitups", "expanded", "gitops-package-set.yaml")) {
		t.Fatalf("ExpandedPackageSet = %q", ws.ExpandedPackageSet)
	}
	if !strings.HasSuffix(ws.RenderRoot, filepath.Join(".gitups", "render")) {
		t.Fatalf("RenderRoot = %q", ws.RenderRoot)
	}
}

func TestScaffoldPackageSetEmbedsName(t *testing.T) {
	got := ScaffoldPackageSet("alpha")
	if !strings.Contains(got, "name: alpha") {
		t.Fatalf("scaffold missing metadata.name; got:\n%s", got)
	}
	if !strings.Contains(got, "kind: GitOpsPackageSet") {
		t.Fatalf("scaffold missing kind line")
	}
}

func TestParseFillSetHappy(t *testing.T) {
	tests := []struct {
		in        string
		wantInst  string
		wantPath  string
		wantValue string
	}{
		{"metallb.values.replicas=3", "metallb", "values.replicas", "3"},
		{"x.a=", "x", "a", ""},
		{"a.b=eq=allowed", "a", "b", "eq=allowed"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			inst, path, val, err := ParseFillSet(tc.in)
			if err != nil {
				t.Fatalf("ParseFillSet: %v", err)
			}
			if inst != tc.wantInst || path != tc.wantPath || val != tc.wantValue {
				t.Fatalf("ParseFillSet(%q) = (%q, %q, %q), want (%q, %q, %q)",
					tc.in, inst, path, val, tc.wantInst, tc.wantPath, tc.wantValue)
			}
		})
	}
}

func TestParseFillSetErrors(t *testing.T) {
	bad := []string{
		"no-equals", // missing =
		".a=v",      // empty instance
		"x.=v",      // empty path
		"=v",        // missing dot AND empty instance
		"x=v",       // missing dot in left side
	}
	for _, tc := range bad {
		t.Run(tc, func(t *testing.T) {
			if _, _, _, err := ParseFillSet(tc); err == nil {
				t.Fatalf("ParseFillSet(%q): expected error", tc)
			}
		})
	}
}

func TestSetDottedPath(t *testing.T) {
	m := map[string]any{}
	if err := SetDottedPath(m, "a.b.c", 42); err != nil {
		t.Fatalf("SetDottedPath: %v", err)
	}
	got := m["a"].(map[string]any)["b"].(map[string]any)["c"]
	if got != 42 {
		t.Fatalf("a.b.c = %v, want 42", got)
	}

	// Overwrite leaf.
	if err := SetDottedPath(m, "a.b.c", "new"); err != nil {
		t.Fatalf("SetDottedPath overwrite: %v", err)
	}
	if m["a"].(map[string]any)["b"].(map[string]any)["c"] != "new" {
		t.Fatalf("overwrite failed")
	}

	// Walking through a non-map segment must fail.
	m2 := map[string]any{"a": "string"}
	if err := SetDottedPath(m2, "a.b", "x"); err == nil {
		t.Fatal("SetDottedPath through string: expected error")
	}
}

func TestRenderedRepoNamesUniqueAndOrdered(t *testing.T) {
	fp := &v1.GitOpsPackageSet{
		Spec: v1.GitOpsPackageSetSpec{
			Resolved: &v1.ResolvedGitOps{
				Packages: []v1.ResolvedPackage{
					{RenderedPaths: v1.RenderedPaths{Repo: "platform"}},
					{RenderedPaths: v1.RenderedPaths{Repo: "platform"}}, // dup
					{RenderedPaths: v1.RenderedPaths{Repo: "addons"}},
					{RenderedPaths: v1.RenderedPaths{Repo: ""}}, // skipped
				},
			},
		},
	}
	got := RenderedRepoNames(fp)
	want := []string{"platform", "addons"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("RenderedRepoNames = %v, want %v", got, want)
	}
}

func TestPlanUnitTag(t *testing.T) {
	tests := []struct {
		name string
		rp   *v1.ResolvedPackage
		want string
	}{
		{
			name: "controller binding wins",
			rp:   &v1.ResolvedPackage{UnitType: v1.UnitTypeInstall, InstallMethod: "olm", Controller: &v1.ControllerBinding{Instance: "krc-x", Intent: "apply"}},
			want: "krc-x/apply",
		},
		{
			name: "install method",
			rp:   &v1.ResolvedPackage{UnitType: v1.UnitTypeInstall, InstallMethod: "helm"},
			want: "install/helm",
		},
		{
			name: "resource KRC self",
			rp:   &v1.ResolvedPackage{UnitType: v1.UnitTypeResource, Role: v1.RoleKRC},
			want: "kubernetes-resource-controller/self",
		},
		{
			name: "resource template",
			rp:   &v1.ResolvedPackage{UnitType: v1.UnitTypeResource, ResourceTemplate: "my-app"},
			want: "resource/my-app",
		},
		{
			name: "unknown unit type falls through to UnitType string",
			rp:   &v1.ResolvedPackage{UnitType: "weird"},
			want: "weird",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := PlanUnitTag(tc.rp); got != tc.want {
				t.Fatalf("PlanUnitTag = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBootstrapSubsetIncludesInstallAndControllers(t *testing.T) {
	fp := &v1.GitOpsPackageSet{
		Spec: v1.GitOpsPackageSetSpec{
			Resolved: &v1.ResolvedGitOps{
				Packages: []v1.ResolvedPackage{
					{Instance: "i-install", UnitType: v1.UnitTypeInstall},
					{Instance: "i-resource", UnitType: v1.UnitTypeResource}, // no controller, no KRC role → skip
					{Instance: "i-bound", UnitType: v1.UnitTypeResource, Controller: &v1.ControllerBinding{Instance: "krc-x", Intent: "apply"}},
					{Instance: "i-krc", UnitType: v1.UnitTypeResource, Role: v1.RoleKRC},
				},
			},
		},
	}
	got := BootstrapSubset(fp)
	names := make([]string, 0, len(got))
	for _, rp := range got {
		names = append(names, rp.Instance)
	}
	want := []string{"i-install", "i-bound", "i-krc"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("BootstrapSubset names = %v, want %v", names, want)
	}
}

func TestWriteBootstrapPlanCount(t *testing.T) {
	fp := &v1.GitOpsPackageSet{
		Metadata: v1.Metadata{Name: "dev"},
		Spec: v1.GitOpsPackageSetSpec{
			Resolved: &v1.ResolvedGitOps{
				Packages: []v1.ResolvedPackage{
					{Instance: "a"},
					{Instance: "b"},
					{Instance: "c"},
				},
			},
		},
	}
	planned := []*v1.ResolvedPackage{
		{Instance: "a", ApplyWave: 0, UnitType: v1.UnitTypeInstall, InstallMethod: "olm", RenderedPaths: v1.RenderedPaths{Repo: "platform"}},
	}
	var out bytes.Buffer
	WriteBootstrapPlan(&out, fp, planned)
	if !strings.Contains(out.String(), "1 direct, 2 deferred") {
		t.Fatalf("WriteBootstrapPlan: expected counts; got %q", out.String())
	}
}

func TestParseMajorMinor(t *testing.T) {
	tests := []struct {
		in      string
		wantMaj int
		wantMin int
	}{
		{"1.30", 1, 30},
		{"1.35+", 1, 35},
		// Leading "v" on the major makes maj=0 (parser stops at 'v') while minor still parses.
		// K8sVersionSatisfies treats maj=0 as "unknown server" and passes any constraint.
		{"v1.28.0", 0, 28},
		{"", 0, 0},
		{"abc.def", 0, 0},
		{"1", 0, 0}, // missing minor segment
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			maj, min := ParseMajorMinor(tc.in)
			if maj != tc.wantMaj || min != tc.wantMin {
				t.Fatalf("ParseMajorMinor(%q) = (%d,%d), want (%d,%d)", tc.in, maj, min, tc.wantMaj, tc.wantMin)
			}
		})
	}
}

func TestK8sVersionSatisfies(t *testing.T) {
	tests := []struct {
		name        string
		server      string
		constraints []string
		want        bool
	}{
		{"unknown server passes anything", "", []string{">=1.30"}, true},
		{">= ok", "1.31", []string{">=1.30"}, true},
		{">= not ok", "1.29", []string{">=1.30"}, false},
		{"<= ok", "1.30", []string{"<=1.30"}, true},
		{"== match", "1.30", []string{"==1.30"}, true},
		{"== mismatch", "1.31", []string{"==1.30"}, false},
		{"compound passes", "1.31", []string{">=1.30", "<=1.40"}, true},
		{"compound fails one", "1.41", []string{">=1.30", "<=1.40"}, false},
		{"malformed constraint ignored", "1.31", []string{"junk"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := K8sVersionSatisfies(tc.server, tc.constraints); got != tc.want {
				t.Fatalf("K8sVersionSatisfies(%q, %v) = %v, want %v", tc.server, tc.constraints, got, tc.want)
			}
		})
	}
}

func TestPackageNamesByTemplate(t *testing.T) {
	prov := &v1.GitOpsPackageSet{
		Spec: v1.GitOpsPackageSetSpec{
			Repositories: []v1.RepositoryDecl{
				{
					Packages: []v1.PackageRef{
						{Template: "local/metallb"},
						{Template: "local/metallb"}, // dup → one entry
						{Template: "local/olm"},
						{Template: "oci/cert-manager"},
						{Template: "no-slash"}, // skipped
					},
				},
			},
		},
	}
	got := PackageNamesByTemplate(prov)
	if names := got["local"]; len(names) != 2 || names[0] != "metallb" || names[1] != "olm" {
		t.Fatalf("local source names = %v, want [metallb olm]", names)
	}
	if names := got["oci"]; len(names) != 1 || names[0] != "cert-manager" {
		t.Fatalf("oci source names = %v, want [cert-manager]", names)
	}
}

func TestPackageNamesFromExpandedPackageSet(t *testing.T) {
	fp := &v1.GitOpsPackageSet{
		Spec: v1.GitOpsPackageSetSpec{
			Resolved: &v1.ResolvedGitOps{
				Packages: []v1.ResolvedPackage{
					{Template: "local/metallb"},
					{Template: "oci/cert-manager"},
					{Template: "malformed"}, // skipped
				},
			},
		},
	}
	got := PackageNamesFromExpandedPackageSet(fp)
	if got["local"][0] != "metallb" || got["oci"][0] != "cert-manager" {
		t.Fatalf("PackageNamesFromExpandedPackageSet = %v", got)
	}
}

func TestSourceCacheDir(t *testing.T) {
	if got := SourceCacheDir("/tmp/state"); got != filepath.Join("/tmp/state", "gitops", "sources") {
		t.Fatalf("SourceCacheDir = %q", got)
	}
}

func TestDiffWorkspaceFindsMissingModifiedExtra(t *testing.T) {
	root := t.TempDir()
	rendered := filepath.Join(root, "rendered")
	workspace := filepath.Join(root, "ws")

	// rendered/platform/{a.yaml, b.yaml}
	writeFile(t, filepath.Join(rendered, "platform", "a.yaml"), "want-a\n")
	writeFile(t, filepath.Join(rendered, "platform", "b.yaml"), "want-b\n")
	// rendered/addons/c.yaml
	writeFile(t, filepath.Join(rendered, "addons", "c.yaml"), "want-c\n")

	// ws/platform/{a.yaml differs, no b.yaml, extra.yaml is extra}
	writeFile(t, filepath.Join(workspace, "platform", "a.yaml"), "have-a\n")
	writeFile(t, filepath.Join(workspace, "platform", "extra.yaml"), "extra\n")
	// ws is missing the "addons" repo entirely → expect missing-dir + missing for the file.
	// ws has an orphan-dir "stale".
	if err := os.MkdirAll(filepath.Join(workspace, "stale"), 0o755); err != nil {
		t.Fatalf("mkdir stale: %v", err)
	}

	drifts, err := DiffWorkspace(workspace, rendered)
	if err != nil {
		t.Fatalf("DiffWorkspace: %v", err)
	}
	kinds := map[string]int{}
	paths := []string{}
	for _, d := range drifts {
		kinds[d.Kind]++
		paths = append(paths, d.Kind+"|"+d.Path)
	}
	if kinds["modified"] != 1 {
		t.Fatalf("expected one modified, got %v (drifts=%v)", kinds["modified"], paths)
	}
	if kinds["missing"] < 1 {
		t.Fatalf("expected at least one missing, got %v (drifts=%v)", kinds["missing"], paths)
	}
	if kinds["missing-dir"] != 1 {
		t.Fatalf("expected one missing-dir, got %v (drifts=%v)", kinds["missing-dir"], paths)
	}
	if kinds["extra"] != 1 {
		t.Fatalf("expected one extra, got %v (drifts=%v)", kinds["extra"], paths)
	}
	if kinds["orphan-dir"] != 1 {
		t.Fatalf("expected one orphan-dir, got %v (drifts=%v)", kinds["orphan-dir"], paths)
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}

func TestAbsPath(t *testing.T) {
	got := AbsPath("relative/path")
	if !filepath.IsAbs(got) {
		t.Fatalf("AbsPath returned non-absolute %q", got)
	}
}

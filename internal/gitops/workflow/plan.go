package workflow

import (
	"fmt"
	"io"
	"strings"

	v1 "github.com/crmarques/gitups/api/v1alpha1"
)

func WriteBootstrapPlan(out io.Writer, fp *v1.GitOpsPackageSet, planned []*v1.ResolvedPackage) {
	out = writerOrDiscard(out)
	total := len(fp.Spec.Resolved.Packages)
	deferred := total - len(planned)
	fmt.Fprintf(out, "gitups: plan — %d direct, %d deferred to KRC (total %d); handoff after direct set succeeds\n",
		len(planned), deferred, total)
	const maxList = 30
	for i, rp := range planned {
		if i == maxList {
			fmt.Fprintf(out, "gitups:   ... (+%d more; run `gitups plan gitops %s` for the full list)\n",
				len(planned)-maxList, fp.Metadata.Name)
			break
		}
		fmt.Fprintf(out, "gitups:   [wave %d] %s (%s) → %s\n",
			rp.ApplyWave, rp.Instance, PlanUnitTag(rp), rp.RenderedPaths.Repo)
	}
}

func PlanUnitTag(rp *v1.ResolvedPackage) string {
	switch {
	case rp.Controller != nil:
		return fmt.Sprintf("%s/%s", rp.Controller.Instance, rp.Controller.Intent)
	case rp.UnitType == v1.UnitTypeInstall:
		return fmt.Sprintf("install/%s", rp.InstallMethod)
	case rp.UnitType == v1.UnitTypeResource:
		if rp.Role == v1.RoleKRC || rp.Role == v1.RoleSRC {
			return fmt.Sprintf("%s/self", rp.Role)
		}
		return fmt.Sprintf("resource/%s", rp.ResourceTemplate)
	}
	return rp.UnitType
}

func BootstrapSubset(fp *v1.GitOpsPackageSet) []*v1.ResolvedPackage {
	var out []*v1.ResolvedPackage
	for i := range fp.Spec.Resolved.Packages {
		rp := &fp.Spec.Resolved.Packages[i]
		switch {
		case rp.UnitType == v1.UnitTypeInstall:
			out = append(out, rp)
		case rp.Controller != nil:
			out = append(out, rp)
		case rp.Role == v1.RoleKRC || rp.Role == v1.RoleSRC:
			out = append(out, rp)
		}
	}
	return out
}

func K8sVersionSatisfies(server string, constraints []string) bool {
	sMaj, sMin := ParseMajorMinor(server)
	if sMaj == 0 {
		return true
	}
	for _, c := range constraints {
		c = strings.TrimSpace(c)
		op, rest := splitOp(c)
		cMaj, cMin := ParseMajorMinor(rest)
		if cMaj == 0 {
			continue
		}
		cmp := (sMaj*1000 + sMin) - (cMaj*1000 + cMin)
		ok := true
		switch op {
		case ">=":
			ok = cmp >= 0
		case ">":
			ok = cmp > 0
		case "<=":
			ok = cmp <= 0
		case "<":
			ok = cmp < 0
		case "==", "":
			ok = cmp == 0
		}
		if !ok {
			return false
		}
	}
	return true
}

func ParseMajorMinor(s string) (maj, min int) {
	parts := strings.SplitN(s, ".", 3)
	if len(parts) < 2 {
		return 0, 0
	}
	for i, p := range parts[:2] {
		n := 0
		for _, r := range p {
			if r < '0' || r > '9' {
				break
			}
			n = n*10 + int(r-'0')
		}
		if i == 0 {
			maj = n
		} else {
			min = n
		}
	}
	return maj, min
}

func splitOp(c string) (op, rest string) {
	for _, o := range []string{">=", "<=", "==", ">", "<"} {
		if strings.HasPrefix(c, o) {
			return o, strings.TrimSpace(c[len(o):])
		}
	}
	return "", c
}

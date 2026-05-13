package workflow

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Drift struct {
	Kind string
	Path string
}

func DiffWorkspace(wsRoot, rendered string) ([]Drift, error) {
	rEntries, err := os.ReadDir(rendered)
	if err != nil {
		return nil, fmt.Errorf("read rendered: %w", err)
	}
	var drifts []Drift
	renderedRepos := map[string]bool{}
	for _, e := range rEntries {
		if !e.IsDir() {
			continue
		}
		renderedRepos[e.Name()] = true
		wsPath := filepath.Join(wsRoot, e.Name())
		rPath := filepath.Join(rendered, e.Name())
		if _, err := os.Stat(wsPath); errors.Is(err, fs.ErrNotExist) {
			drifts = append(drifts, Drift{Kind: "missing-dir", Path: e.Name() + "/"})
		}
		if err := compareRepoTree(rPath, wsPath, e.Name(), &drifts); err != nil {
			return nil, err
		}
	}
	wsEntries, err := os.ReadDir(wsRoot)
	if err == nil {
		for _, e := range wsEntries {
			if !e.IsDir() {
				continue
			}
			if !renderedRepos[e.Name()] {
				drifts = append(drifts, Drift{Kind: "orphan-dir", Path: e.Name() + "/"})
			}
		}
	}
	sort.SliceStable(drifts, func(i, j int) bool {
		if drifts[i].Path == drifts[j].Path {
			return drifts[i].Kind < drifts[j].Kind
		}
		return drifts[i].Path < drifts[j].Path
	})
	return drifts, nil
}

func WriteDriftDiff(out io.Writer, want, have string, maxLines int) {
	wantBody, werr := os.ReadFile(want)
	haveBody, herr := os.ReadFile(have)
	if werr != nil || herr != nil {
		return
	}
	wantLines := strings.Split(string(wantBody), "\n")
	haveLines := strings.Split(string(haveBody), "\n")
	var lines []string
	n := len(wantLines)
	if len(haveLines) < n {
		n = len(haveLines)
	}
	start := 0
	for start < n && wantLines[start] == haveLines[start] {
		start++
	}
	for i := start; i < len(wantLines); i++ {
		if maxLines > 0 && i >= start+maxLines {
			lines = append(lines, fmt.Sprintf("      ... (+%d more lines in want)", len(wantLines)-i))
			break
		}
		lines = append(lines, fmt.Sprintf("    - %s", wantLines[i]))
	}
	for i := start; i < len(haveLines); i++ {
		if maxLines > 0 && i >= start+maxLines {
			lines = append(lines, fmt.Sprintf("      ... (+%d more lines in have)", len(haveLines)-i))
			break
		}
		lines = append(lines, fmt.Sprintf("    + %s", haveLines[i]))
	}
	for _, l := range lines {
		fmt.Fprintln(out, l)
	}
}

func compareRepoTree(rendered, workspace, prefix string, drifts *[]Drift) error {
	rFiles := map[string]bool{}
	err := filepath.WalkDir(rendered, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(rendered, p)
		rFiles[rel] = true
		displayPath := filepath.Join(prefix, rel)
		wsFile := filepath.Join(workspace, rel)
		wsBody, werr := os.ReadFile(wsFile)
		if errors.Is(werr, fs.ErrNotExist) {
			*drifts = append(*drifts, Drift{Kind: "missing", Path: displayPath})
			return nil
		}
		if werr != nil {
			return werr
		}
		rBody, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		if !bytes.Equal(rBody, wsBody) {
			*drifts = append(*drifts, Drift{Kind: "modified", Path: displayPath})
		}
		return nil
	})
	if err != nil {
		return err
	}
	if _, err := os.Stat(workspace); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return filepath.WalkDir(workspace, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(workspace, p)
		if !rFiles[rel] {
			*drifts = append(*drifts, Drift{Kind: "extra", Path: filepath.Join(prefix, rel)})
		}
		return nil
	})
}

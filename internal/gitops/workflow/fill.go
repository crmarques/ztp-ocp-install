package workflow

import (
	"fmt"
	"strings"
)

func ParseFillSet(s string) (instance, path, value string, err error) {
	eq := strings.IndexByte(s, '=')
	if eq < 0 {
		return "", "", "", fmt.Errorf("missing '='")
	}
	left := s[:eq]
	value = s[eq+1:]
	dot := strings.IndexByte(left, '.')
	if dot < 0 {
		return "", "", "", fmt.Errorf("missing '.' between <instance> and <path>")
	}
	instance = left[:dot]
	path = left[dot+1:]
	if instance == "" || path == "" {
		return "", "", "", fmt.Errorf("instance and path are both required")
	}
	return instance, path, value, nil
}

func SetDottedPath(m map[string]any, path string, v any) error {
	parts := strings.Split(path, ".")
	cur := m
	for i, p := range parts {
		if i == len(parts)-1 {
			cur[p] = v
			return nil
		}
		next, ok := cur[p]
		if !ok {
			child := map[string]any{}
			cur[p] = child
			cur = child
			continue
		}
		nm, ok := next.(map[string]any)
		if !ok {
			return fmt.Errorf("path %q: segment %q is not a map", path, p)
		}
		cur = nm
	}
	return nil
}

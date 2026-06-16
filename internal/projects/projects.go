// Package projects discovers git repositories under a scan root.
package projects

import (
	"os"
	"path/filepath"
	"sort"
)

// Project is a discovered repository fleet can create sessions for.
type Project struct {
	Name          string
	Path          string
	DefaultBranch string
}

// Inspector is the subset of git operations the scanner needs.
type Inspector interface {
	IsRepo(path string) bool
	DefaultBranch(path string) (string, error)
}

// Scan returns the git repositories that are immediate children of root,
// sorted by name. Non-repo entries and files are ignored.
func Scan(root string, g Inspector) ([]Project, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var out []Project
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(root, e.Name())
		if !g.IsRepo(path) {
			continue
		}
		branch, err := g.DefaultBranch(path)
		if err != nil {
			branch = ""
		}
		out = append(out, Project{Name: e.Name(), Path: path, DefaultBranch: branch})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

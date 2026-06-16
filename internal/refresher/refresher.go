// Package refresher rebuilds the live list of sessions from the filesystem
// (the set of worktrees with meta), tmux (liveness), and git (status).
package refresher

import (
	"os"
	"path/filepath"

	"github.com/bray/fleet/internal/config"
	"github.com/bray/fleet/internal/git"
	"github.com/bray/fleet/internal/meta"
	"github.com/bray/fleet/internal/naming"
	"github.com/bray/fleet/internal/session"
)

// liveness is the subset of tmux used here.
type liveness interface{ Has(name string) bool }

// Build returns all sessions discovered under cfg.WorktreeBaseDir. Worktrees
// whose meta cannot be read are skipped (degrade gracefully).
func Build(cfg config.Config, t liveness, g git.Git) ([]session.Session, error) {
	base := cfg.WorktreeBaseDir
	projectDirs, err := os.ReadDir(base)
	if os.IsNotExist(err) {
		return nil, nil // nothing created yet
	}
	if err != nil {
		return nil, err
	}

	var out []session.Session
	for _, pd := range projectDirs {
		if !pd.IsDir() {
			continue
		}
		sessDirs, err := os.ReadDir(filepath.Join(base, pd.Name()))
		if err != nil {
			continue
		}
		for _, sd := range sessDirs {
			if !sd.IsDir() {
				continue
			}
			wt := filepath.Join(base, pd.Name(), sd.Name())
			md, err := meta.Read(wt)
			if err != nil {
				continue // not a fleet worktree, or malformed — skip
			}
			tname := naming.TmuxName(md.Project, md.Session)
			alive := t.Has(tname)
			st, err := g.Status(wt)
			if err != nil {
				st = git.Status{Branch: md.Branch}
			}
			out = append(out, session.Session{
				Project:      md.Project,
				Name:         md.Session,
				Branch:       md.Branch,
				Base:         md.Base,
				RepoPath:     md.RepoPath,
				WorktreePath: wt,
				TmuxName:     tname,
				CreatedAt:    md.CreatedAt,
				Alive:        alive,
				Exited:       !alive,
				Git:          st,
			})
		}
	}
	return out, nil
}

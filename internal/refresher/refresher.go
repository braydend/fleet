// Package refresher rebuilds the live list of sessions from the filesystem
// (the set of worktrees with meta), tmux (window liveness + activity), and git
// (status).
package refresher

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bray/fleet/internal/activity"
	"github.com/bray/fleet/internal/config"
	"github.com/bray/fleet/internal/git"
	"github.com/bray/fleet/internal/meta"
	"github.com/bray/fleet/internal/naming"
	"github.com/bray/fleet/internal/session"
	"github.com/bray/fleet/internal/tmux"
)

// workspaceTmux is the subset of the tmux adapter the refresher needs.
type workspaceTmux interface {
	ListWindows() ([]tmux.Window, error)
	CapturePane(target string) (string, error)
	SetWindowLabel(target, label string) error
}

// Build returns all sessions discovered under cfg.WorktreeBaseDir. Worktrees
// whose meta cannot be read are skipped. now supplies the clock for activity
// classification (pass time.Now in production).
func Build(cfg config.Config, t workspaceTmux, g git.Git, now func() time.Time) ([]session.Session, error) {
	base := cfg.WorktreeBaseDir
	projectDirs, err := os.ReadDir(base)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Index live windows by their stable name.
	windows := map[string]tmux.Window{}
	if wins, err := t.ListWindows(); err == nil {
		for _, w := range wins {
			windows[w.Name] = w
		}
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
				continue
			}
			wname := naming.TmuxName(md.Project, md.Session)
			target := naming.WindowTarget(md.Project, md.Session)
			w, present := windows[wname]
			alive := present && !w.Dead

			var tail string
			if alive {
				tail, _ = t.CapturePane(target) // best-effort
			}
			state := activity.Classify(w.LastActivity, now(), tail, !present, w.Dead)

			st, err := g.Status(wt)
			if err != nil {
				st = git.Status{Branch: md.Branch}
			}

			s := session.Session{
				Project:      md.Project,
				Name:         md.Session,
				Branch:       md.Branch,
				Base:         md.Base,
				RepoPath:     md.RepoPath,
				WorktreePath: wt,
				TmuxName:     wname,
				CreatedAt:    md.CreatedAt,
				Alive:        alive,
				Exited:       !alive,
				Activity:     state,
				LastActivity: w.LastActivity,
				WindowIndex:  w.Index,
				Git:          st,
			}
			out = append(out, s)

			if alive {
				_ = t.SetWindowLabel(target, tabLabel(s)) // best-effort
			}
		}
	}
	return out, nil
}

// tabLabel builds the tmux tab text for a session: a coloured glyph, the
// project/session name, and a dirty marker.
func tabLabel(s session.Session) string {
	dirty := ""
	if s.Git.Dirty {
		dirty = fmt.Sprintf("✱%d", s.Git.ChangeCount)
	}
	return fmt.Sprintf("#[fg=%s]%s#[default] %s/%s%s",
		s.Activity.TmuxColor(), s.Activity.Glyph(), s.Project, s.Name, dirty)
}

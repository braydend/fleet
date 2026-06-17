# fleet usage guide

The complete reference for installing, configuring, and operating `fleet`. For a
quick start see the [README](../README.md).

## Dependencies

Required on your `PATH` at runtime:

| Tool | Purpose |
|------|---------|
| `git` | worktree and branch operations, status queries |
| `tmux` | runs each Claude Code instance in its own session |
| `claude` | the [Claude Code](https://claude.com/claude-code) CLI that each session launches |

Optional:

| Tool | Purpose |
|------|---------|
| `gh` | opening a pull request from the cleanup menu (push still works without it) |

To build from source you also need **Go 1.22+**.

`fleet` checks for `git`, `tmux`, and `claude` at startup and exits with a clear
message if any are missing.

## Install

### Download a release

Prebuilt binaries for Linux and macOS (x64 and ARM) are attached to each
[GitHub Release](https://github.com/braydend/fleet/releases). Download the
archive for your platform, then:

```bash
tar -xzf fleet_<version>_<os>_<arch>.tar.gz
sudo mv fleet /usr/local/bin/   # or anywhere on your PATH
fleet --version
```

**macOS:** the binaries are unsigned, so Gatekeeper will block the first run.
Clear the quarantine flag once after extracting:

```bash
xattr -d com.apple.quarantine ./fleet
```

(Alternatively, right-click the binary in Finder and choose **Open** the first
time.)

### Build from source

Clone and build the binary:

```bash
git clone https://github.com/braydend/fleet.git
cd fleet
go build -o fleet .
```

Then put `fleet` somewhere on your `PATH` (e.g. `~/.local/bin`), or run it from
the build directory. During development you can also run it directly with
`go run .`.

## Configuration

`fleet` reads `~/.config/fleet/config.yaml`.

**First run:** if the file doesn't exist, `fleet` prompts you for the directory
to scan for git projects and writes the config for you (using the default
worktree location). You don't have to create it by hand.

To configure it manually instead, create the file yourself:

```yaml
# Directory scanned (one level deep) for git repositories to offer as projects.
scan_root: /home/you/code

# Where session worktrees are created: <worktree_base_dir>/<project>/<session>.
worktree_base_dir: /home/you/.local/share/fleet/worktrees
```

- `scan_root` is **required** (the first-run prompt collects it; it must be an
  existing directory).
- `worktree_base_dir` defaults to `~/.local/share/fleet/worktrees` if omitted.

## Keybindings

**Dashboard**

| Key | Action |
|-----|--------|
| `n` | new session (opens the project picker) |
| `Enter` | attach to the selected session |
| `d` | cleanup menu for the selected session |
| `r` | refresh now |
| `↑`/`k`, `↓`/`j` | move selection |
| `q` / `Ctrl-c` | quit |

**Project picker / cleanup menu**: `↑`/`↓` to move, `Enter` to choose, `Esc` to cancel.

**New-session form**: `Tab`/`Shift-Tab` (or `↑`/`↓`) to move between fields, type
to edit, `Enter` to advance / submit on the last field, `Esc` to cancel. The
branch defaults to `fleet/<session>` and the base to the project's default branch.

## Cleanup menu

The cleanup menu (`d` on the dashboard) offers three actions for a session:

- **delete worktree + branch** — kills the tmux session and removes the worktree
  (and branch). If the worktree has uncommitted or unpushed changes, you'll be
  asked to confirm.
- **push / open PR** — pushes the branch and, if `gh` is available, opens a PR.
- **leave** — kills only the tmux session, leaving the worktree and branch for
  you to handle manually.

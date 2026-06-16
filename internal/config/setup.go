package config

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Exists reports whether a config file is present at path.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Save writes c to path as YAML, creating the parent directory if needed.
func Save(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// Setup runs first-run configuration: it prompts (on out) for the scan root,
// reads one line from in, validates it, and writes a config file at path
// layered over the built-in defaults. The resulting config is returned. The
// file is only written once a valid scan_root has been provided.
func Setup(path string, in io.Reader, out io.Writer) (Config, error) {
	cfg := Default()

	fmt.Fprintf(out, "No config found at %s.\n", path)
	fmt.Fprint(out, "Enter the directory to scan for git projects (scan_root): ")

	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		return Config{}, fmt.Errorf("reading scan_root: %w", err)
	}

	scanRoot := expandHome(strings.TrimSpace(line))
	if scanRoot == "" {
		return Config{}, errors.New("scan_root is required")
	}
	info, statErr := os.Stat(scanRoot)
	if statErr != nil || !info.IsDir() {
		return Config{}, fmt.Errorf("scan_root %q is not an existing directory", scanRoot)
	}

	cfg.ScanRoot = scanRoot
	if err := Save(path, cfg); err != nil {
		return Config{}, err
	}
	fmt.Fprintf(out, "Wrote config to %s\n", path)
	return cfg, nil
}

// expandHome replaces a leading "~" with the user's home directory.
func expandHome(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if p == "~" {
		return home
	}
	return filepath.Join(home, p[2:])
}

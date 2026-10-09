// Package kekkai loads workspace domains: their config, env, skills, workflows
// and the directories steps may run in.
package kekkai

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Worker is one named worker in kekkai.yaml.
type Worker struct {
	Provider       string   `yaml:"provider" json:"provider"` // claude | codex | fake
	Bin            string   `yaml:"bin" json:"bin,omitempty"` // claude/codex binary; default from PATH (e.g. claude to follow the active account)
	Model          string   `yaml:"model" json:"model,omitempty"`
	Effort         string   `yaml:"effort" json:"effort,omitempty"`
	Command        string   `yaml:"command" json:"command,omitempty"` // fake provider: script run as the worker
	Tools          []string `yaml:"tools" json:"tools,omitempty"`
	PermissionMode string   `yaml:"permission_mode" json:"permission_mode,omitempty"`
}

// Mode is an AFK mode as written in config: kind attended | grace | afk.
type Mode struct {
	Kind  string `yaml:"kind" json:"kind"`
	Grace string `yaml:"grace" json:"grace,omitempty"`
}

// Kekkai is one loaded domain.
type Kekkai struct {
	Name              string            `yaml:"name" json:"name"`
	Description       string            `yaml:"description" json:"description,omitempty"`
	EnvFile           string            `yaml:"env_file" json:"env_file,omitempty"`
	Prompt            string            `yaml:"prompt" json:"prompt,omitempty"`
	Skills            []string          `yaml:"skills" json:"skills,omitempty"`
	CwdRoots          []string          `yaml:"cwd_roots" json:"cwd_roots,omitempty"`
	Workers           map[string]Worker `yaml:"workers" json:"workers,omitempty"`
	AFKDefault        *Mode             `yaml:"afk_default" json:"afk_default,omitempty"`
	MementoNamespaces []string          `yaml:"memento_namespaces" json:"memento_namespaces,omitempty"`

	Dir string            `yaml:"-" json:"dir"`
	Env map[string]string `yaml:"-" json:"-"`
}

// Root is the directory holding every kekkai: $KAMI_HOME/kekkai.
func Root(home string) string { return filepath.Join(home, "kekkai") }

// Load reads $home/kekkai/<name>/kekkai.yaml and its env file.
func Load(home, name string) (*Kekkai, error) {
	if name == "" || strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return nil, fmt.Errorf("invalid kekkai name %q", name)
	}
	return LoadDir(filepath.Join(Root(home), name), name)
}

// LoadDir loads the kekkai in dir; name must match kekkai.yaml when set.
func LoadDir(dir, name string) (*Kekkai, error) {
	if name == "" {
		name = filepath.Base(dir)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "kekkai.yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("kekkai %q not found (no %s)", name, filepath.Join(dir, "kekkai.yaml"))
		}
		return nil, err
	}
	k := &Kekkai{}
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(k); err != nil {
		return nil, fmt.Errorf("kekkai %s: kekkai.yaml: %w", name, err)
	}
	if k.Name == "" {
		k.Name = name
	}
	if k.Name != name && filepath.Base(dir) == name {
		return nil, fmt.Errorf("kekkai %s: kekkai.yaml says name %q", name, k.Name)
	}
	k.Dir = dir
	k.Env = map[string]string{}
	if k.EnvFile != "" {
		env, err := ParseEnv(filepath.Join(dir, k.EnvFile))
		if err != nil {
			return nil, fmt.Errorf("kekkai %s: %w", name, err)
		}
		k.Env = env
	}
	for n, w := range k.Workers {
		if w.Bin != "" {
			w.Bin = ExpandHome(w.Bin)
			k.Workers[n] = w
		}
	}
	for i, r := range k.CwdRoots {
		k.CwdRoots[i] = ExpandHome(r)
	}
	return k, nil
}

// List returns the names of every kekkai under home.
func List(home string) ([]string, error) {
	ents, err := os.ReadDir(Root(home))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range ents {
		if _, err := os.Stat(filepath.Join(Root(home), e.Name(), "kekkai.yaml")); err == nil {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

// ParseEnv reads a dotenv file: KEY=VALUE lines, # comments, optional quotes.
func ParseEnv(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	env := map[string]string{}
	sc := bufio.NewScanner(f)
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			return nil, fmt.Errorf("%s:%d: expected KEY=VALUE", path, n)
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
			v = v[1 : len(v)-1]
		}
		env[k] = v
	}
	return env, sc.Err()
}

// ExpandHome turns a leading ~ into the user's home directory.
func ExpandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// CheckCwd resolves dir (after ~ expansion and symlinks) and refuses it unless
// it sits under one of the kekkai's cwd_roots.
func (k *Kekkai) CheckCwd(dir string) (string, error) {
	dir = ExpandHome(dir)
	if !filepath.IsAbs(dir) {
		return "", fmt.Errorf("cwd %q must be absolute", dir)
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("cwd %q: %w", dir, err)
	}
	for _, r := range k.CwdRoots {
		rr, err := filepath.EvalSymlinks(r)
		if err != nil {
			continue
		}
		if real == rr || strings.HasPrefix(real, rr+string(filepath.Separator)) {
			return real, nil
		}
	}
	return "", fmt.Errorf("cwd %q is outside kekkai %s cwd_roots %v", dir, k.Name, k.CwdRoots)
}

// WorkerEnv builds the only environment a worker or script in this kekkai gets:
// the kekkai env file, a minimal set of process variables, and extra.
func (k *Kekkai) WorkerEnv(extra map[string]string) []string {
	m := map[string]string{}
	// CLAUDE_CONFIG_DIR / CODEX_HOME pick the logged-in account; without them
	// claude falls back to ~/.claude, which may not be logged in.
	for _, key := range []string{"PATH", "HOME", "USER", "LOGNAME", "LANG", "TMPDIR", "TERM", "SHELL", "CLAUDE_CONFIG_DIR", "CODEX_HOME"} {
		if v, ok := os.LookupEnv(key); ok {
			m[key] = v
		}
	}
	for key, v := range k.Env {
		m[key] = v
	}
	for key, v := range extra {
		m[key] = v
	}
	out := make([]string, 0, len(m))
	for key, v := range m {
		out = append(out, key+"="+v)
	}
	sort.Strings(out)
	return out
}

// Config is $KAMI_HOME/config.yaml.
type Config struct {
	Active  string `yaml:"active"`
	Memento string `yaml:"memento_namespace"`
}

// LoadConfig reads $home/config.yaml; a missing file is an empty config.
func LoadConfig(home string) (*Config, error) {
	c := &Config{}
	raw, err := os.ReadFile(filepath.Join(home, "config.yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return nil, err
	}
	if err := yaml.Unmarshal(raw, c); err != nil {
		return nil, fmt.Errorf("config.yaml: %w", err)
	}
	return c, nil
}

// SaveConfig writes $home/config.yaml atomically.
func SaveConfig(home string, c *Config) error {
	raw, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		return err
	}
	tmp := filepath.Join(home, ".config.yaml.tmp")
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(home, "config.yaml"))
}

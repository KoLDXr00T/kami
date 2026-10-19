package kekkai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadEnvAndCwd(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "kekkai", "k")
	root := filepath.Join(home, "root")
	outside := filepath.Join(home, "outside")
	for _, d := range []string{dir, filepath.Join(root, "sub"), outside} {
		os.MkdirAll(d, 0o755)
	}
	os.WriteFile(filepath.Join(dir, "kekkai.yaml"), []byte("name: k\nenv_file: env\ncwd_roots: ["+root+"]\n"), 0o600)
	os.WriteFile(filepath.Join(dir, "env"), []byte("# c\nA=1\nexport B=\"two words\"\nC='x=y'\nPATH=/tools::rel\n"), 0o600)
	os.Symlink(outside, filepath.Join(root, "escape"))

	k, err := Load(home, "k")
	if err != nil {
		t.Fatal(err)
	}
	if k.Env["A"] != "1" || k.Env["B"] != "two words" || k.Env["C"] != "x=y" {
		t.Fatalf("env = %v", k.Env)
	}
	if _, err := k.CheckCwd(filepath.Join(root, "sub")); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{outside, filepath.Join(root, "escape"), root + "sibling", "relative"} {
		if _, err := k.CheckCwd(bad); err == nil {
			t.Errorf("CheckCwd(%s) allowed", bad)
		}
	}
	env := strings.Join(k.WorkerEnv(map[string]string{"X": "y"}), "\n")
	if !strings.Contains(env, "A=1") || !strings.Contains(env, "X=y") {
		t.Fatalf("worker env = %s", env)
	}
	if env := strings.Join(k.WorkerEnv(map[string]string{"PATH": "/engine"}), "\n"); !strings.Contains(env, "PATH=/engine:/tools\n") {
		t.Fatalf("kekkai PATH not after the engine PATH: %s", env)
	}
	t.Setenv("LEAKY_SECRET", "z")
	if strings.Contains(strings.Join(k.WorkerEnv(nil), "\n"), "LEAKY_SECRET") {
		t.Fatal("process env leaked into the worker env")
	}
	if _, err := Load(home, "../k"); err == nil {
		t.Fatal("path traversal in a kekkai name was accepted")
	}
}

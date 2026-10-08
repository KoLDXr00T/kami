package kamui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExamplesValidate(t *testing.T) {
	for _, k := range []string{"acme", "suno"} {
		dir := filepath.Join("..", "..", "examples", "kekkai", k)
		files, _ := filepath.Glob(filepath.Join(dir, "workflows", "*.star"))
		if len(files) == 0 {
			t.Fatalf("no workflows under %s", dir)
		}
		for _, f := range files {
			rel, _ := filepath.Rel(dir, f)
			w, err := Load(filepath.ToSlash(rel), DirReader(dir))
			if err != nil {
				t.Errorf("%s: %v", rel, err)
				continue
			}
			if p := Validate(w, nil); len(p) > 0 {
				t.Errorf("%s:\n  %s", rel, strings.Join(p, "\n  "))
			}
		}
	}
}

func load(t *testing.T, src string) (*Workflow, error) {
	t.Helper()
	files := map[string]string{"workflows/t.star": src, "lib/x.star": `
def two(then):
    state("a", script("scripts/a"), ok = "b")
    state("b", script("scripts/b"), ok = then)
`}
	return Load("workflows/t.star", BundleReader(files))
}

func TestValidateCatches(t *testing.T) {
	cases := map[string]string{
		`"nope", which does not exist`: `
workflow(name = "t", start = "a")
state("a", script("x"), ok = "nope")`,
		`unreachable states: b`: `
workflow(name = "t", start = "a")
state("a", done())
state("b", done())`,
		`needs timeout=`: `
workflow(name = "t", start = "a")
state("a", wait(signal = "go"), signal = "z")
state("z", done())`,
		`answered`: `
workflow(name = "t", start = "a")
state("a", ask("q?"), yes = "z")
state("z", done())`,
		`choose can return "ghost"`: `
workflow(name = "t", start = "a")
def g(v):
    return "z" if v["x"] else "ghost"
state("a", choose(g))
state("z", done())`,
	}
	for want, src := range cases {
		w, err := load(t, src)
		if err != nil {
			t.Fatalf("%s: load: %v", want, err)
		}
		p := strings.Join(Validate(w, nil), "\n")
		if !strings.Contains(p, want) {
			t.Errorf("want problem containing %q, got:\n%s", want, p)
		}
	}
}

func TestLoadErrors(t *testing.T) {
	cases := map[string]string{
		"declared twice": `
workflow(name = "t", start = "a")
state("a", done())
state("a", done())`,
		"only a human": `
workflow(name = "t", start = "a")
state("a", gate("ok?", resolver = "r"), go = "a")`,
		"never called": `x = 1`,
	}
	for want, src := range cases {
		_, err := load(t, src)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want error containing %q, got %v", want, err)
		}
	}
}

func TestLoadFragmentAndChoose(t *testing.T) {
	w, err := load(t, `
load("lib/x.star", "two")
workflow(name = "t", start = "a", entries = {"late": entry("b", require = ["k"])})
two(then = "pick")
def pick(v):
    if v["visits"]["a"] > 1:
        return "end"
    return "a"
state("pick", choose(pick))
state("end", done("bye {who}"))
`)
	if err != nil {
		t.Fatal(err)
	}
	if p := Validate(w, nil); len(p) > 0 {
		t.Fatalf("problems: %v", p)
	}
	if len(w.Files) != 2 || w.Hash == "" {
		t.Fatalf("bundle not recorded: %v", w.Files)
	}
	got, err := Call(w.States["pick"].Step.Fn, map[string]any{"visits": map[string]any{"a": 2}})
	if err != nil || got != "end" {
		t.Fatalf("pick = %v, %v", got, err)
	}
	if _, err := w.Next("a", "ok"); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Next("a", "nope"); err == nil {
		t.Fatal("want error for unknown outcome")
	}
}

func TestRenderAndSplit(t *testing.T) {
	s, miss := Render("{a} {b.c} {{x}} {n} {gone}", map[string]any{"a": "A", "b": map[string]any{"c": 1.0}, "n": []any{"x"}})
	if s != `A 1 {x} ["x"] ` || len(miss) != 1 || miss[0] != "gone" {
		t.Fatalf("render = %q missing %v", s, miss)
	}
	args, err := SplitCommand(`scripts/x {a} "two words" 'q'`)
	if err != nil || strings.Join(args, "|") != "scripts/x|{a}|two words|q" {
		t.Fatalf("split = %v %v", args, err)
	}
}

func TestCron(t *testing.T) {
	loc := time.UTC
	at := func(s string) time.Time { tt, _ := time.ParseInLocation("2006-01-02 15:04:05", s, loc); return tt }
	cases := []struct{ expr, from, want string }{
		{"0 9 * * *", "2026-10-08 09:00:00", "2026-10-09 09:00:00"},
		{"*/2 * * * * *", "2026-10-08 09:00:01", "2026-10-08 09:00:02"},
		{"30 8 * * 1-5", "2026-10-09 09:00:00", "2026-10-12 08:30:00"}, // Fri → Mon
		{"0 0 1 * *", "2026-12-15 00:00:00", "2027-01-01 00:00:00"},
	}
	for _, c := range cases {
		cr, err := ParseCron(c.expr)
		if err != nil {
			t.Fatal(err)
		}
		if got := cr.Next(at(c.from)); !got.Equal(at(c.want)) {
			t.Errorf("%s after %s = %s, want %s", c.expr, c.from, got, c.want)
		}
	}
	for _, bad := range []string{"* * *", "61 * * * *", "*/0 * * * *"} {
		if _, err := ParseCron(bad); err == nil {
			t.Errorf("%q should not parse", bad)
		}
	}
}

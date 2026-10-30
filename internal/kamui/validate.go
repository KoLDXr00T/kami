package kamui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"kami/internal/kekkai"
)

var inputTypes = map[string]bool{"string": true, "dict": true, "list": true, "bool": true, "int": true, "number": true, "any": true}

// ValidType reports whether t is a known input or field type.
func ValidType(t string) bool {
	t = strings.TrimSuffix(t, "?")
	if strings.HasPrefix(t, "enum:") {
		return len(strings.TrimPrefix(t, "enum:")) > 0
	}
	return inputTypes[t]
}

// Validate checks a loaded workflow. k may be nil, which skips the checks
// that need the kekkai (workers, skills, cwd roots, command files).
func Validate(w *Workflow, k *kekkai.Kekkai) []string {
	var p []string
	add := func(f string, a ...any) { p = append(p, fmt.Sprintf(f, a...)) }

	if _, ok := w.States[w.Start]; !ok {
		add("start state %q does not exist", w.Start)
	}
	for n, e := range w.Entries {
		if _, ok := w.States[e.State]; !ok {
			add("entry %q starts at %q, which does not exist", n, e.State)
		}
	}
	for n, t := range w.Inputs {
		if !ValidType(t) {
			add("input %q has unknown type %q", n, t)
		}
	}
	for _, name := range w.Order {
		st := w.States[name]
		s := st.Step
		where := fmt.Sprintf("state %q", name)
		for _, o := range st.Order {
			if _, ok := w.States[st.Outcomes[o]]; !ok {
				add("%s: outcome %s goes to %q, which does not exist", where, o, st.Outcomes[o])
			}
		}
		has := func(o string) bool { _, ok := st.Outcomes[o]; return ok }
		switch {
		case s.IsTerminal() || s.Kind == KChoose:
			if len(st.Outcomes) > 0 {
				add("%s: %s takes no outcomes", where, s.Kind)
			}
		case s.Kind == KAsk:
			if !has("answered") {
				add("%s: ask needs an answered = <state> outcome", where)
			}
		case s.Kind == KSpawn:
			if !has("all_done") || !has("any_failed") {
				add("%s: spawn needs all_done = and any_failed = outcomes", where)
			}
		case s.Kind == KWait:
			if s.Signal != "" && !has("signal") {
				add("%s: wait on signal needs a signal = <state> outcome", where)
			}
			if s.Timer > 0 && !has("timer") {
				add("%s: wait with timer needs a timer = <state> outcome", where)
			}
			if (s.Signal != "" || s.Probe != "") && s.Timeout == 0 {
				add("%s: a wait on a signal or probe needs timeout=", where)
			}
			if s.Timeout > 0 && !has("timeout") {
				add("%s: wait with timeout needs a timeout = <state> outcome", where)
			}
			if s.Probe != "" && len(st.Outcomes) == boolInt(has("timeout"))+boolInt(has("timer"))+boolInt(has("signal")) {
				add("%s: probe needs at least one outcome besides timeout", where)
			}
			if has("pending") {
				add("%s: pending is how a probe says \"keep waiting\"; it cannot be an outcome", where)
			}
		default:
			if len(st.Outcomes) == 0 {
				add("%s: %s needs at least one outcome", where, s.Kind)
			}
		}
		for o := range s.Say {
			if o != "" && !has(o) {
				add("%s: say names outcome %q, which the state does not have", where, o)
			}
		}
		for f, t := range s.Fields {
			if !ValidType(t) {
				add("%s: field %q has unknown type %q", where, f, t)
			}
		}
		if k != nil {
			p = append(p, checkKekkai(w, st, k)...)
		}
	}

	// reachability from start and every entry, following choose functions
	reach := map[string]bool{}
	var walk func(string)
	walk = func(s string) {
		st, ok := w.States[s]
		if !ok || reach[s] {
			return
		}
		reach[s] = true
		for _, o := range st.Order {
			walk(st.Outcomes[o])
		}
		if st.Step.Kind == KChoose {
			targets, complete := ChooseTargets(w, st.Step.Fn)
			if !complete {
				add("state %q: cannot see every state choose(...) returns; return string literals so validation can follow it", s)
			}
			for _, t := range targets {
				if _, ok := w.States[t]; !ok {
					add("state %q: choose can return %q, which does not exist", s, t)
				}
				walk(t)
			}
		}
	}
	walk(w.Start)
	for _, e := range w.Entries {
		walk(e.State)
	}
	var un []string
	for _, n := range w.Order {
		if !reach[n] {
			un = append(un, n)
		}
	}
	if len(un) > 0 {
		add("unreachable states: %s", strings.Join(un, ", "))
	}
	sort.Strings(p)
	return p
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func checkKekkai(w *Workflow, st *State, k *kekkai.Kekkai) []string {
	var p []string
	add := func(f string, a ...any) { p = append(p, fmt.Sprintf("state %q: ", st.Name)+fmt.Sprintf(f, a...)) }
	s := st.Step
	if s.Worker != "" {
		if _, ok := k.Workers[s.Worker]; !ok {
			add("worker %q is not in kekkai %s workers", s.Worker, k.Name)
		}
	}
	for _, sk := range s.Skills {
		if _, err := os.Stat(filepath.Join(k.Dir, "skills", sk, "SKILL.md")); err != nil {
			add("skill %q not found at skills/%s/SKILL.md", sk, sk)
		}
	}
	if s.Cwd != "" && !strings.Contains(s.Cwd, "{") {
		if _, err := k.CheckCwd(s.Cwd); err != nil {
			add("%v", err)
		}
	}
	for _, c := range []string{s.Cmd, s.Probe, s.Resolver, s.Heartbeat} {
		if c == "" {
			continue
		}
		args, err := SplitCommand(c)
		if err != nil {
			add("%v", err)
			continue
		}
		if CommandIsLocal(args[0]) && !strings.Contains(args[0], "{") {
			if _, err := os.Stat(filepath.Join(k.Dir, args[0])); err != nil {
				add("command %s not found in the kekkai", args[0])
			}
		}
	}
	if s.Kind == KSpawn {
		if _, err := os.Stat(filepath.Join(k.Dir, WorkflowPath(s.Workflow))); err != nil {
			add("spawns workflow %q, but %s does not exist", s.Workflow, WorkflowPath(s.Workflow))
		}
	}
	if w.AFK != nil && w.AFK.Resolver != "" && st.Name == w.Start {
		if args, err := SplitCommand(w.AFK.Resolver); err == nil && CommandIsLocal(args[0]) {
			if _, err := os.Stat(filepath.Join(k.Dir, args[0])); err != nil {
				p = append(p, fmt.Sprintf("workflow afk resolver %s not found in the kekkai", args[0]))
			}
		}
	}
	return p
}

// CommandIsLocal reports whether a command's first word is a path inside the
// kekkai ("scripts/x") rather than a program on PATH ("make").
func CommandIsLocal(arg0 string) bool {
	return strings.Contains(arg0, "/") && !filepath.IsAbs(arg0) && !strings.HasPrefix(arg0, "~")
}

// ChooseTargets lists the string literals fn can return, by reading its source.
// complete is false when some return is not a literal (or a conditional of them).
func ChooseTargets(w *Workflow, fn starlark.Callable) (targets []string, complete bool) {
	f, ok := fn.(*starlark.Function)
	if !ok {
		return nil, false
	}
	pos := f.Position()
	src, ok := w.Files[pos.Filename()]
	if !ok {
		return nil, false
	}
	file, err := (&syntax.FileOptions{}).Parse(pos.Filename(), src, 0)
	if err != nil {
		return nil, false
	}
	complete = true
	var leaves func(e syntax.Expr)
	leaves = func(e syntax.Expr) {
		switch x := e.(type) {
		case *syntax.Literal:
			if s, ok := x.Value.(string); ok {
				targets = append(targets, s)
				return
			}
			complete = false
		case *syntax.CondExpr:
			leaves(x.True)
			leaves(x.False)
		case *syntax.ParenExpr:
			leaves(x.X)
		default:
			complete = false
		}
	}
	found := false
	syntax.Walk(file, func(n syntax.Node) bool {
		if found {
			return false
		}
		switch x := n.(type) {
		case *syntax.DefStmt:
			if x.Def.Line == pos.Line || x.Name.NamePos.Line == pos.Line {
				found = true
				syntax.Walk(x, func(n syntax.Node) bool {
					if _, nested := n.(*syntax.LambdaExpr); nested {
						return false
					}
					if r, ok := n.(*syntax.ReturnStmt); ok {
						if r.Result == nil {
							complete = false
						} else {
							leaves(r.Result)
						}
					}
					return true
				})
				return false
			}
		case *syntax.LambdaExpr:
			if x.Lambda.Line == pos.Line {
				found = true
				leaves(x.Body)
				return false
			}
		}
		return true
	})
	if !found {
		return nil, false
	}
	return targets, complete
}

// ReachableWithoutHuman reports whether a run in from can get to to on its own: following
// outcomes and choose functions, it may arrive at a human-only state but not pass through one.
func (w *Workflow) ReachableWithoutHuman(from, to string) bool {
	seen := map[string]bool{}
	var walk func(string) bool
	walk = func(s string) bool {
		if s == to {
			return true
		}
		st := w.States[s]
		if seen[s] || st == nil || st.Step.HumanOnly() {
			return false
		}
		seen[s] = true
		for _, n := range st.Outcomes {
			if walk(n) {
				return true
			}
		}
		if st.Step.Kind == KChoose {
			targets, _ := ChooseTargets(w, st.Step.Fn)
			for _, n := range targets {
				if walk(n) {
					return true
				}
			}
		}
		return false
	}
	return walk(from)
}

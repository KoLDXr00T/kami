package kamui

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// ReadFile reads a path relative to the kekkai root.
type ReadFile func(p string) ([]byte, error)

// DirReader reads files under dir, refusing paths that escape it.
func DirReader(dir string) ReadFile {
	return func(p string) ([]byte, error) {
		clean := path.Clean(p)
		if path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
			return nil, fmt.Errorf("load %q: path escapes the kekkai", p)
		}
		return os.ReadFile(filepath.Join(dir, filepath.FromSlash(clean)))
	}
}

// BundleReader serves files from a pinned bundle.
func BundleReader(files map[string]string) ReadFile {
	return func(p string) ([]byte, error) {
		src, ok := files[path.Clean(p)]
		if !ok {
			return nil, fmt.Errorf("%s: not in the pinned bundle", p)
		}
		return []byte(src), nil
	}
}

// WorkflowPath is where a workflow named name lives inside a kekkai.
func WorkflowPath(name string) string { return "workflows/" + name + ".star" }

const localWF = "kami.workflow"

// Load executes entry (relative to the kekkai root) and returns its workflow.
func Load(entry string, read ReadFile) (*Workflow, error) {
	w := &Workflow{
		Entry:   path.Clean(entry),
		Files:   map[string]string{},
		States:  map[string]*State{},
		Entries: map[string]Entry{},
		Inputs:  map[string]string{},
		Resume:  "aura",
	}
	opts := &syntax.FileOptions{}
	modules := map[string]starlark.StringDict{}
	var loading map[string]bool = map[string]bool{}
	pre := builtins()
	var exec func(t *starlark.Thread, p string) (starlark.StringDict, error)
	exec = func(t *starlark.Thread, p string) (starlark.StringDict, error) {
		p = path.Clean(p)
		if g, ok := modules[p]; ok {
			return g, nil
		}
		if loading[p] {
			return nil, fmt.Errorf("load cycle through %s", p)
		}
		loading[p] = true
		src, err := read(p)
		if err != nil {
			return nil, err
		}
		w.Files[p] = string(src)
		g, err := starlark.ExecFileOptions(opts, t, p, src, pre)
		if err != nil {
			return nil, err
		}
		modules[p] = g
		return g, nil
	}
	t := &starlark.Thread{Name: "load " + entry}
	t.Load = func(t *starlark.Thread, module string) (starlark.StringDict, error) { return exec(t, module) }
	t.SetLocal(localWF, w)
	if _, err := exec(t, w.Entry); err != nil {
		if ee, ok := err.(*starlark.EvalError); ok {
			return nil, fmt.Errorf("%s", ee.Backtrace())
		}
		return nil, err
	}
	if w.Name == "" {
		return nil, fmt.Errorf("%s: workflow(...) was never called", entry)
	}
	h := sha256.New()
	keys := make([]string, 0, len(w.Files))
	for k := range w.Files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(h, "%s\x00%d\x00%s", k, len(w.Files[k]), w.Files[k])
	}
	w.Hash = hex.EncodeToString(h.Sum(nil))
	return w, nil
}

// --- builtins ----------------------------------------------------------------

type stepValue struct{ s *Step }

func (v stepValue) String() string        { return "<step " + v.s.Kind + ">" }
func (v stepValue) Type() string          { return "step" }
func (v stepValue) Freeze()               {}
func (v stepValue) Truth() starlark.Bool  { return true }
func (v stepValue) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: step") }

type afkValue struct{ a *AFK }

func (v afkValue) String() string        { return "<afk " + v.a.String() + ">" }
func (v afkValue) Type() string          { return "afk" }
func (v afkValue) Freeze()               {}
func (v afkValue) Truth() starlark.Bool  { return true }
func (v afkValue) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: afk") }

type entryValue struct{ e Entry }

func (v entryValue) String() string        { return "<entry " + v.e.State + ">" }
func (v entryValue) Type() string          { return "entry" }
func (v entryValue) Freeze()               {}
func (v entryValue) Truth() starlark.Bool  { return true }
func (v entryValue) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: entry") }

type triggerValue struct{ t Trigger }

func (v triggerValue) String() string        { return "<cron " + v.t.Cron + ">" }
func (v triggerValue) Type() string          { return "cron" }
func (v triggerValue) Freeze()               {}
func (v triggerValue) Truth() starlark.Bool  { return true }
func (v triggerValue) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: cron") }

func wfOf(t *starlark.Thread) (*Workflow, error) {
	w, ok := t.Local(localWF).(*Workflow)
	if !ok {
		return nil, fmt.Errorf("workflow and state can only be called while the file loads, not from a guard")
	}
	return w, nil
}

func builtins() starlark.StringDict {
	b := starlark.StringDict{
		"workflow": starlark.NewBuiltin("workflow", bWorkflow),
		"state":    starlark.NewBuiltin("state", bState),
		"agent":    starlark.NewBuiltin("agent", bAgent),
		"script":   starlark.NewBuiltin("script", bScript),
		"wait":     starlark.NewBuiltin("wait", bWait),
		"spawn":    starlark.NewBuiltin("spawn", bSpawn),
		"choose":   starlark.NewBuiltin("choose", bChoose),
		"afk":      starlark.NewBuiltin("afk", bAFK),
		"entry":    starlark.NewBuiltin("entry", bEntry),
		"cron":     starlark.NewBuiltin("cron", bCron),
	}
	for _, k := range []string{KDone, KFail, KCancel} {
		b[k] = starlark.NewBuiltin(k, bTerminal(k))
	}
	for _, k := range []string{KAsk, KChoice, KForm, KBlock, KGate, KAction} {
		b[k] = starlark.NewBuiltin(k, bContract(k))
	}
	return b
}

func bWorkflow(t *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kw []starlark.Tuple) (starlark.Value, error) {
	w, err := wfOf(t)
	if err != nil {
		return nil, err
	}
	if w.Name != "" {
		return nil, fmt.Errorf("workflow: called twice")
	}
	var name, start string
	var inputs, entries *starlark.Dict
	var afk, triggers starlark.Value
	resume := "aura"
	maxVisits := 5
	if err := starlark.UnpackArgs("workflow", args, kw, "name", &name, "start", &start,
		"inputs?", &inputs, "afk?", &afk, "resume?", &resume, "max_visits?", &maxVisits,
		"entries?", &entries, "triggers?", &triggers); err != nil {
		return nil, err
	}
	if name == "" {
		return nil, fmt.Errorf("workflow: name is empty")
	}
	w.Name, w.Start, w.Resume, w.MaxVisits = name, start, resume, maxVisits
	if resume != "aura" && resume != "session" {
		return nil, fmt.Errorf("workflow: resume must be \"aura\" or \"session\", got %q", resume)
	}
	if inputs != nil {
		for _, it := range inputs.Items() {
			k, ok1 := starlark.AsString(it[0])
			v, ok2 := starlark.AsString(it[1])
			if !ok1 || !ok2 {
				return nil, fmt.Errorf("workflow: inputs must map names to type strings")
			}
			w.Inputs[k] = v
		}
	}
	if afk != nil && afk != starlark.None {
		a, err := asAFK("workflow", afk)
		if err != nil {
			return nil, err
		}
		w.AFK = a
	}
	if entries != nil {
		for _, it := range entries.Items() {
			k, _ := starlark.AsString(it[0])
			ev, ok := it[1].(entryValue)
			if !ok {
				return nil, fmt.Errorf("workflow: entries[%q] must be entry(...)", k)
			}
			w.Entries[k] = ev.e
		}
	}
	if triggers != nil && triggers != starlark.None {
		l, ok := triggers.(*starlark.List)
		if !ok {
			return nil, fmt.Errorf("workflow: triggers must be a list of cron(...)")
		}
		for i := 0; i < l.Len(); i++ {
			tv, ok := l.Index(i).(triggerValue)
			if !ok {
				return nil, fmt.Errorf("workflow: triggers[%d] must be cron(...)", i)
			}
			w.Triggers = append(w.Triggers, tv.t)
		}
	}
	return starlark.None, nil
}

func bState(t *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kw []starlark.Tuple) (starlark.Value, error) {
	w, err := wfOf(t)
	if err != nil {
		return nil, err
	}
	if len(args) != 2 {
		return nil, fmt.Errorf("state: want state(name, step, outcome = next, ...), got %d positional args", len(args))
	}
	name, ok := starlark.AsString(args[0])
	if !ok || name == "" {
		return nil, fmt.Errorf("state: name must be a non-empty string")
	}
	sv, ok := args[1].(stepValue)
	if !ok {
		return nil, fmt.Errorf("state %q: second argument must be a step (agent, script, ask, ...), got %s", name, args[1].Type())
	}
	if _, dup := w.States[name]; dup {
		return nil, fmt.Errorf("state %q declared twice", name)
	}
	st := &State{Name: name, Step: sv.s, Outcomes: map[string]string{}}
	if fr := t.CallFrame(1); fr.Pos.IsValid() {
		st.Pos = fr.Pos.String()
	}
	for _, p := range kw {
		k := string(p[0].(starlark.String))
		v, ok := starlark.AsString(p[1])
		if !ok {
			return nil, fmt.Errorf("state %q: outcome %s must name a state (a string)", name, k)
		}
		st.Outcomes[k] = v
		st.Order = append(st.Order, k)
	}
	w.States[name] = st
	w.Order = append(w.Order, name)
	return starlark.None, nil
}

func dur(fn, field string, v starlark.Value) (time.Duration, error) {
	if v == nil || v == starlark.None {
		return 0, nil
	}
	s, ok := starlark.AsString(v)
	if !ok {
		return 0, fmt.Errorf("%s: %s must be a duration string like \"10m\"", fn, field)
	}
	return ParseDuration(s)
}

func strList(fn, field string, v starlark.Value) ([]string, error) {
	if v == nil || v == starlark.None {
		return nil, nil
	}
	l, ok := v.(*starlark.List)
	if !ok {
		return nil, fmt.Errorf("%s: %s must be a list of strings", fn, field)
	}
	out := make([]string, 0, l.Len())
	for i := 0; i < l.Len(); i++ {
		s, ok := starlark.AsString(l.Index(i))
		if !ok {
			return nil, fmt.Errorf("%s: %s[%d] must be a string", fn, field, i)
		}
		out = append(out, s)
	}
	return out, nil
}

// parseSay accepts say = "text" (every outcome) or say = {"outcome": "text"}.
func parseSay(fn string, v starlark.Value) (map[string]string, error) {
	switch x := v.(type) {
	case nil, starlark.NoneType:
		return nil, nil
	case starlark.String:
		return map[string]string{"": string(x)}, nil
	case *starlark.Dict:
		out := map[string]string{}
		for _, it := range x.Items() {
			k, ok1 := starlark.AsString(it[0])
			t, ok2 := starlark.AsString(it[1])
			if !ok1 || !ok2 {
				return nil, fmt.Errorf("%s: say must map outcomes to strings", fn)
			}
			out[k] = t
		}
		return out, nil
	}
	return nil, fmt.Errorf("%s: say must be a string or a dict of outcome -> string", fn)
}

func optStr(v starlark.Value) string {
	if s, ok := starlark.AsString(v); ok {
		return s
	}
	return ""
}

func bAgent(t *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kw []starlark.Tuple) (starlark.Value, error) {
	var worker starlark.Value
	var instruction string
	var skills, inputs, cwd, timeout, resume, say starlark.Value
	maxVisits := 0
	if err := starlark.UnpackArgs("agent", args, kw, "worker", &worker, "instruction", &instruction,
		"skills?", &skills, "inputs?", &inputs, "cwd?", &cwd, "timeout?", &timeout,
		"resume?", &resume, "max_visits?", &maxVisits, "say?", &say); err != nil {
		return nil, err
	}
	s := &Step{Kind: KAgent, Instruction: instruction, MaxVisits: maxVisits, Cwd: optStr(cwd), Resume: optStr(resume)}
	switch wv := worker.(type) {
	case starlark.String:
		s.Worker = string(wv)
	case starlark.Callable:
		s.WorkerFn = wv
	default:
		return nil, fmt.Errorf("agent: worker must be a name or a function of vars")
	}
	var err error
	if s.Skills, err = strList("agent", "skills", skills); err != nil {
		return nil, err
	}
	if inputs != nil && inputs != starlark.None {
		s.InputsSet = true
		if s.Inputs, err = strList("agent", "inputs", inputs); err != nil {
			return nil, err
		}
	}
	if s.Timeout, err = dur("agent", "timeout", timeout); err != nil {
		return nil, err
	}
	if s.Resume != "" && s.Resume != "aura" && s.Resume != "session" {
		return nil, fmt.Errorf("agent: resume must be \"aura\" or \"session\"")
	}
	if s.Say, err = parseSay("agent", say); err != nil {
		return nil, err
	}
	return stepValue{s}, nil
}

func bScript(t *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kw []starlark.Tuple) (starlark.Value, error) {
	var cmd string
	var inputs, cwd, timeout, say starlark.Value
	maxVisits := 0
	if err := starlark.UnpackArgs("script", args, kw, "cmd", &cmd, "inputs?", &inputs, "cwd?", &cwd,
		"timeout?", &timeout, "max_visits?", &maxVisits, "say?", &say); err != nil {
		return nil, err
	}
	s := &Step{Kind: KScript, Cmd: cmd, Cwd: optStr(cwd), MaxVisits: maxVisits}
	var err error
	if inputs != nil && inputs != starlark.None {
		s.InputsSet = true
		if s.Inputs, err = strList("script", "inputs", inputs); err != nil {
			return nil, err
		}
	}
	if s.Timeout, err = dur("script", "timeout", timeout); err != nil {
		return nil, err
	}
	if _, err := SplitCommand(cmd); err != nil {
		return nil, fmt.Errorf("script: %w", err)
	}
	if s.Say, err = parseSay("script", say); err != nil {
		return nil, err
	}
	return stepValue{s}, nil
}

func bWait(t *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kw []starlark.Tuple) (starlark.Value, error) {
	var signal, key, timer, probe, every, timeout, cwd, say starlark.Value
	maxVisits, maxErrors := 0, 0
	if err := starlark.UnpackArgs("wait", args, kw, "signal?", &signal, "key?", &key, "timer?", &timer,
		"probe?", &probe, "every?", &every, "timeout?", &timeout, "cwd?", &cwd, "max_visits?", &maxVisits, "say?", &say,
		"max_errors?", &maxErrors); err != nil {
		return nil, err
	}
	s := &Step{Kind: KWait, Signal: optStr(signal), Key: optStr(key), Probe: optStr(probe), Cwd: optStr(cwd), MaxVisits: maxVisits,
		MaxErrors: maxErrors}
	var err error
	if s.Timer, err = dur("wait", "timer", timer); err != nil {
		return nil, err
	}
	if s.Every, err = dur("wait", "every", every); err != nil {
		return nil, err
	}
	if s.Timeout, err = dur("wait", "timeout", timeout); err != nil {
		return nil, err
	}
	if s.Signal == "" && s.Timer == 0 && s.Probe == "" {
		return nil, fmt.Errorf("wait: needs signal=, timer= or probe=")
	}
	if maxErrors < 0 || (maxErrors > 0 && s.Probe == "") {
		return nil, fmt.Errorf("wait: max_errors= needs probe= and a count of at least 1")
	}
	if s.Probe != "" {
		if s.Every == 0 {
			return nil, fmt.Errorf("wait: probe= needs every=")
		}
		if _, err := SplitCommand(s.Probe); err != nil {
			return nil, fmt.Errorf("wait: probe: %w", err)
		}
	}
	if s.Say, err = parseSay("wait", say); err != nil {
		return nil, err
	}
	return stepValue{s}, nil
}

func bSpawn(t *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kw []starlark.Tuple) (starlark.Value, error) {
	var workflow string
	var forEach, input, say starlark.Value
	maxVisits := 0
	if err := starlark.UnpackArgs("spawn", args, kw, "workflow", &workflow, "for_each", &forEach,
		"input?", &input, "max_visits?", &maxVisits, "say?", &say); err != nil {
		return nil, err
	}
	s := &Step{Kind: KSpawn, Workflow: workflow, MaxVisits: maxVisits}
	switch v := forEach.(type) {
	case starlark.String:
		s.ForEach = string(v)
	case starlark.Callable:
		s.ForEachFn = v
	default:
		return nil, fmt.Errorf("spawn: for_each must be a var name or a function of vars")
	}
	if input != nil && input != starlark.None {
		c, ok := input.(starlark.Callable)
		if !ok {
			return nil, fmt.Errorf("spawn: input must be a function (item, vars) -> dict")
		}
		s.InputFn = c
	}
	var err error
	if s.Say, err = parseSay("spawn", say); err != nil {
		return nil, err
	}
	return stepValue{s}, nil
}

func bChoose(t *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kw []starlark.Tuple) (starlark.Value, error) {
	var f starlark.Callable
	if err := starlark.UnpackPositionalArgs("choose", args, kw, 1, &f); err != nil {
		return nil, err
	}
	return stepValue{&Step{Kind: KChoose, Fn: f}}, nil
}

func bTerminal(kind string) func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
	return func(t *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kw []starlark.Tuple) (starlark.Value, error) {
		report := ""
		if err := starlark.UnpackArgs(kind, args, kw, "report?", &report); err != nil {
			return nil, err
		}
		return stepValue{&Step{Kind: kind, Report: report}}, nil
	}
}

func bContract(kind string) func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
	return func(t *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kw []starlark.Tuple) (starlark.Value, error) {
		var title string
		var body, bodyFile, fields, resolver, mode, grace, def, say starlark.Value
		lock := false
		maxVisits := 0
		if err := starlark.UnpackArgs(kind, args, kw, "title", &title, "body?", &body, "body_file?", &bodyFile,
			"fields?", &fields, "resolver?", &resolver, "mode?", &mode, "grace?", &grace, "lock?", &lock,
			"default?", &def, "max_visits?", &maxVisits, "say?", &say); err != nil {
			return nil, err
		}
		s := &Step{Kind: kind, Title: title, BodyFile: optStr(bodyFile), Resolver: optStr(resolver),
			Lock: lock, Default: optStr(def), MaxVisits: maxVisits}
		switch b := body.(type) {
		case nil, starlark.NoneType:
		case starlark.String:
			s.Body = string(b)
		case starlark.Callable:
			s.BodyFn = b
		default:
			return nil, fmt.Errorf("%s: body must be a string or a function of vars", kind)
		}
		if fields != nil && fields != starlark.None {
			d, ok := fields.(*starlark.Dict)
			if !ok {
				return nil, fmt.Errorf("%s: fields must be a dict of name -> type", kind)
			}
			s.Fields = map[string]string{}
			for _, it := range d.Items() {
				k, ok1 := starlark.AsString(it[0])
				v, ok2 := starlark.AsString(it[1])
				if !ok1 || !ok2 {
					return nil, fmt.Errorf("%s: fields must map names to type strings", kind)
				}
				s.Fields[k] = v
			}
		}
		if mode != nil && mode != starlark.None {
			var a *AFK
			var err error
			if ms, ok := starlark.AsString(mode); ok && ms == "grace" {
				g, gerr := dur(kind, "grace", grace)
				if gerr != nil {
					return nil, gerr
				}
				a = &AFK{Kind: "grace", Grace: g}
			} else if a, err = asAFK(kind, mode); err != nil {
				return nil, err
			}
			s.Mode = a
		}
		if s.HumanOnly() && (s.Resolver != "" || (s.Mode != nil && s.Mode.Kind != "attended")) {
			return nil, fmt.Errorf("%s %q: only a human can resolve it, so it takes no resolver or AFK mode", kind, title)
		}
		var err error
		if s.Say, err = parseSay(kind, say); err != nil {
			return nil, err
		}
		return stepValue{s}, nil
	}
}

func asAFK(fn string, v starlark.Value) (*AFK, error) {
	switch x := v.(type) {
	case afkValue:
		return x.a, nil
	case starlark.String:
		return ParseMode(string(x))
	}
	return nil, fmt.Errorf("%s: mode must be \"attended\", \"afk\", \"grace=10m\" or afk(...)", fn)
}

func bAFK(t *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kw []starlark.Tuple) (starlark.Value, error) {
	var kind string
	var grace, resolver starlark.Value
	if err := starlark.UnpackArgs("afk", args, kw, "kind", &kind, "grace?", &grace, "resolver?", &resolver); err != nil {
		return nil, err
	}
	a := &AFK{Kind: kind, Resolver: optStr(resolver)}
	switch kind {
	case "attended", "afk":
	case "grace":
		g, err := dur("afk", "grace", grace)
		if err != nil {
			return nil, err
		}
		if g == 0 {
			return nil, fmt.Errorf("afk: grace needs grace=\"<duration>\"")
		}
		a.Grace = g
	default:
		return nil, fmt.Errorf("afk: kind must be attended, grace or afk, got %q", kind)
	}
	return afkValue{a}, nil
}

func bEntry(t *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kw []starlark.Tuple) (starlark.Value, error) {
	var st string
	var req starlark.Value
	if err := starlark.UnpackArgs("entry", args, kw, "state", &st, "require?", &req); err != nil {
		return nil, err
	}
	r, err := strList("entry", "require", req)
	if err != nil {
		return nil, err
	}
	return entryValue{Entry{State: st, Require: r}}, nil
}

func bCron(t *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kw []starlark.Tuple) (starlark.Value, error) {
	var expr string
	var input *starlark.Dict
	if err := starlark.UnpackArgs("cron", args, kw, "expr", &expr, "input?", &input); err != nil {
		return nil, err
	}
	c, err := ParseCron(expr)
	if err != nil {
		return nil, fmt.Errorf("cron: %w", err)
	}
	tr := Trigger{Cron: expr, Sched: c}
	if input != nil {
		v, err := FromStarlark(input)
		if err != nil {
			return nil, fmt.Errorf("cron: input: %w", err)
		}
		tr.Input = v.(map[string]any)
	}
	return triggerValue{tr}, nil
}

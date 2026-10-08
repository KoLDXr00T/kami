package kamui

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"go.starlark.net/starlark"
)

// ToStarlark converts JSON-shaped Go values into frozen Starlark values.
func ToStarlark(v any) (starlark.Value, error) {
	switch x := v.(type) {
	case nil:
		return starlark.None, nil
	case bool:
		return starlark.Bool(x), nil
	case string:
		return starlark.String(x), nil
	case int:
		return starlark.MakeInt(x), nil
	case int64:
		return starlark.MakeInt64(x), nil
	case float64:
		if x == math.Trunc(x) && math.Abs(x) < 1<<53 {
			return starlark.MakeInt64(int64(x)), nil
		}
		return starlark.Float(x), nil
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return starlark.MakeInt64(i), nil
		}
		f, err := x.Float64()
		return starlark.Float(f), err
	case []any:
		l := make([]starlark.Value, 0, len(x))
		for _, e := range x {
			sv, err := ToStarlark(e)
			if err != nil {
				return nil, err
			}
			l = append(l, sv)
		}
		out := starlark.NewList(l)
		out.Freeze()
		return out, nil
	case []string:
		a := make([]any, len(x))
		for i, s := range x {
			a[i] = s
		}
		return ToStarlark(a)
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		d := starlark.NewDict(len(x))
		for _, k := range keys {
			sv, err := ToStarlark(x[k])
			if err != nil {
				return nil, err
			}
			d.SetKey(starlark.String(k), sv)
		}
		d.Freeze()
		return d, nil
	case map[string]int:
		m := map[string]any{}
		for k, n := range x {
			m[k] = n
		}
		return ToStarlark(m)
	}
	// anything else round-trips through JSON
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("cannot pass %T to starlark", v)
	}
	var g any
	if err := json.Unmarshal(raw, &g); err != nil {
		return nil, err
	}
	return ToStarlark(g)
}

// FromStarlark converts a Starlark value back to JSON-shaped Go values.
func FromStarlark(v starlark.Value) (any, error) {
	switch x := v.(type) {
	case starlark.NoneType:
		return nil, nil
	case starlark.Bool:
		return bool(x), nil
	case starlark.String:
		return string(x), nil
	case starlark.Int:
		if i, ok := x.Int64(); ok {
			return float64(i), nil
		}
		return nil, fmt.Errorf("integer too large")
	case starlark.Float:
		return float64(x), nil
	case *starlark.List:
		out := make([]any, 0, x.Len())
		for i := 0; i < x.Len(); i++ {
			e, err := FromStarlark(x.Index(i))
			if err != nil {
				return nil, err
			}
			out = append(out, e)
		}
		return out, nil
	case starlark.Tuple:
		out := make([]any, 0, len(x))
		for _, e := range x {
			g, err := FromStarlark(e)
			if err != nil {
				return nil, err
			}
			out = append(out, g)
		}
		return out, nil
	case *starlark.Dict:
		out := map[string]any{}
		for _, it := range x.Items() {
			k, ok := starlark.AsString(it[0])
			if !ok {
				return nil, fmt.Errorf("dict keys must be strings, got %s", it[0].Type())
			}
			e, err := FromStarlark(it[1])
			if err != nil {
				return nil, err
			}
			out[k] = e
		}
		return out, nil
	}
	return nil, fmt.Errorf("cannot convert %s to JSON", v.Type())
}

// Call runs a workflow function (guard, input mapper, body, worker picker) on
// a fresh thread. It gets no builtins that touch the world, and a step budget.
func Call(fn starlark.Callable, args ...any) (any, error) {
	t := &starlark.Thread{Name: "guard"}
	t.SetMaxExecutionSteps(1_000_000)
	sargs := make(starlark.Tuple, len(args))
	for i, a := range args {
		v, err := ToStarlark(a)
		if err != nil {
			return nil, err
		}
		sargs[i] = v
	}
	out, err := starlark.Call(t, fn, sargs, nil)
	if err != nil {
		if ee, ok := err.(*starlark.EvalError); ok {
			return nil, fmt.Errorf("%s", ee.Backtrace())
		}
		return nil, err
	}
	return FromStarlark(out)
}

var tmplRe = regexp.MustCompile(`\{\{|\}\}|\{([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z0-9_]+)*)\}`)

// Lookup resolves a dotted name ("task.title") in vars.
func Lookup(vars map[string]any, name string) (any, bool) {
	var cur any = vars
	for _, part := range strings.Split(name, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		if cur, ok = m[part]; !ok {
			return nil, false
		}
	}
	return cur, true
}

// Render fills {name} from vars. {{ and }} are literal braces. Missing names
// render empty and are returned so the caller can record them.
func Render(s string, vars map[string]any) (string, []string) {
	var missing []string
	out := tmplRe.ReplaceAllStringFunc(s, func(m string) string {
		switch m {
		case "{{":
			return "{"
		case "}}":
			return "}"
		}
		name := m[1 : len(m)-1]
		v, ok := Lookup(vars, name)
		if !ok {
			missing = append(missing, name)
			return ""
		}
		return Stringify(v)
	})
	return out, missing
}

// Stringify renders a var: strings as-is, everything else as JSON.
func Stringify(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		if x == math.Trunc(x) && math.Abs(x) < 1<<53 {
			return fmt.Sprintf("%d", int64(x))
		}
	}
	raw, _ := json.Marshal(v)
	return string(raw)
}

// SplitCommand splits a command template into arguments, honouring single and
// double quotes. Templates are rendered per argument afterwards, so a var can
// never inject extra arguments or shell syntax.
func SplitCommand(s string) ([]string, error) {
	var args []string
	var cur strings.Builder
	inArg := false
	var quote rune
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
			inArg = true
		case r == ' ' || r == '\t' || r == '\n':
			if inArg {
				args = append(args, cur.String())
				cur.Reset()
				inArg = false
			}
		default:
			cur.WriteRune(r)
			inArg = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote in %q", s)
	}
	if inArg {
		args = append(args, cur.String())
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	return args, nil
}

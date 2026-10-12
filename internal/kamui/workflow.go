// Package kamui is the deterministic control plane: it loads Starlark workflow
// definitions, validates them, and answers "which state comes next".
package kamui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.starlark.net/starlark"
)

// Workflow is one loaded .star definition.
type Workflow struct {
	Name      string
	Inputs    map[string]string // name -> type ("string", "string?", "dict", ...)
	Start     string
	AFK       *AFK
	Resume    string // "aura" (default) or "session"
	MaxVisits int
	Entries   map[string]Entry
	Triggers  []Trigger
	States    map[string]*State
	Order     []string

	Entry string            // entry file path, relative to the kekkai root
	Files map[string]string // every file read while loading: path -> source
	Hash  string            // sha256 over Files; a run pins this
}

// State is one state(...) call.
type State struct {
	Name     string
	Step     *Step
	Outcomes map[string]string // outcome -> next state
	Order    []string          // outcome names in declaration order
	Pos      string
}

// Step kinds.
const (
	KAgent  = "agent"
	KScript = "script"
	KWait   = "wait"
	KSpawn  = "spawn"
	KChoose = "choose"
	KDone   = "done"
	KFail   = "fail"
	KCancel = "cancel"
	KAsk    = "ask"
	KChoice = "choice"
	KForm   = "form"
	KBlock  = "blocker"
	KGate   = "gate"
	KAction = "action"
)

// Step is what a state runs.
type Step struct {
	Kind      string
	MaxVisits int
	// Say is a one-line milestone emitted (event run.say) when the step
	// finishes: "" for all outcomes, or keyed by outcome.
	Say map[string]string

	// agent
	Worker      string
	WorkerFn    starlark.Callable
	Instruction string
	Skills      []string
	Inputs      []string
	InputsSet   bool
	Cwd         string
	Timeout     time.Duration
	Resume      string

	// script (Cmd) and wait probe (Probe)
	Cmd string

	// wait
	Signal string
	Key    string
	Timer  time.Duration
	Probe  string
	Every  time.Duration

	// spawn
	Workflow  string
	ForEach   string
	ForEachFn starlark.Callable
	InputFn   starlark.Callable

	// choose
	Fn starlark.Callable

	// done / fail / cancel
	Report string

	// decisions
	Title    string
	Body     string
	BodyFn   starlark.Callable
	BodyFile string
	Fields   map[string]string
	Resolver string
	Mode     *AFK
	Lock     bool
	Default  string
}

// IsContract reports whether the step parks a decision in the contract queue.
func (s *Step) IsContract() bool {
	switch s.Kind {
	case KAsk, KChoice, KForm, KBlock, KGate, KAction:
		return true
	}
	return false
}

// IsTerminal reports whether the step ends the run.
func (s *Step) IsTerminal() bool {
	return s.Kind == KDone || s.Kind == KFail || s.Kind == KCancel
}

// HumanOnly reports whether no resolver may ever answer this decision.
func (s *Step) HumanOnly() bool { return s.Kind == KGate || s.Kind == KAction || s.Lock }

// AFK is an AFK mode: kind attended | grace | afk, plus the resolver to use.
type AFK struct {
	Kind     string        `json:"kind"`
	Grace    time.Duration `json:"grace,omitempty"`
	Resolver string        `json:"resolver,omitempty"`
}

func (a *AFK) String() string {
	if a == nil {
		return "attended"
	}
	if a.Kind == "grace" {
		return "grace=" + a.Grace.String()
	}
	return a.Kind
}

// ParseMode parses "attended", "afk", "grace=10m".
func ParseMode(s string) (*AFK, error) {
	switch {
	case s == "attended" || s == "afk":
		return &AFK{Kind: s}, nil
	case strings.HasPrefix(s, "grace="):
		d, err := ParseDuration(strings.TrimPrefix(s, "grace="))
		if err != nil {
			return nil, err
		}
		if d == 0 {
			return &AFK{Kind: "afk"}, nil
		}
		return &AFK{Kind: "grace", Grace: d}, nil
	}
	return nil, fmt.Errorf("mode %q: want attended, afk or grace=<duration>", s)
}

// Entry is a named way into a workflow below its start state.
type Entry struct {
	State   string
	Require []string
}

// Trigger starts a new run on a schedule.
type Trigger struct {
	Cron  string
	Sched *Cron
	Input map[string]any
}

// ParseDuration accepts Go durations plus a trailing "d" for days.
func ParseDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, fmt.Errorf("duration %q: %w", s, err)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("duration %q: %w", s, err)
	}
	return d, nil
}

// Next returns the state an outcome leads to, or an error naming the allowed set.
func (w *Workflow) Next(state, outcome string) (string, error) {
	st, ok := w.States[state]
	if !ok {
		return "", fmt.Errorf("no state %q", state)
	}
	next, ok := st.Outcomes[outcome]
	if !ok {
		return "", fmt.Errorf("state %q has no outcome %q (has: %s)", state, outcome, strings.Join(st.Order, ", "))
	}
	return next, nil
}

// MaxVisitsFor is the visit budget of a state.
func (w *Workflow) MaxVisitsFor(state string) int {
	if st := w.States[state]; st != nil && st.Step.MaxVisits > 0 {
		return st.Step.MaxVisits
	}
	if w.MaxVisits > 0 {
		return w.MaxVisits
	}
	return 5
}

// ContractOptions are the answers a decision state accepts.
func (s *State) ContractOptions() []string {
	if s.Step.Kind == KAsk {
		return nil // free text; the outcome is always "answered"
	}
	return s.Order
}

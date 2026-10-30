package engine

import (
	"strings"

	"kami/internal/kamui"
)

// BySystem is the resolver name on a contract answered by a system instruction.
const BySystem = "system"

// System is an instruction from outside to a run: it continues where it is, with a log and
// nobody pressing anything. With answer it answers the open contract; with state or vars it
// moves onto the latest workflow (as Upgrade); with neither it runs a waiting probe now. Text
// is the reason, kept as a system.instruction event and a run.say in the run's thread.
// A locked contract stays the owner's: answering it, or the upgrade that would cancel it, is refused,
// and so is a state only reachable by passing through a human-only step.
func (e *Engine) System(id, text, state, answer string, vars map[string]any) (*Run, error) {
	if strings.TrimSpace(text) == "" {
		return nil, uerr("INVALID", "a system instruction needs its text: it is the log")
	}
	if answer != "" && (state != "" || len(vars) > 0) {
		return nil, uerr("INVALID", "--answer goes alone: the run continues from the answer")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	r, err := e.findRun(id)
	if err != nil {
		return nil, err
	}
	if Terminal(r.Status) {
		return nil, uerr("CONFLICT", "run %s is already %s", r.ID, r.Status)
	}
	var open *Contract
	for _, c := range e.contracts {
		if c.RunID != r.ID || c.Status != "open" {
			continue
		}
		if c.Lock {
			return nil, uerr("FORBIDDEN", "run %s waits on locked contract %s: only the owner can answer it", r.ID, c.ID)
		}
		if r.Step != nil && r.Step.Contract == c.ID {
			open = c
		}
	}
	switch {
	case answer != "":
		if open == nil {
			return nil, uerr("CONFLICT", "run %s has no open contract to answer", r.ID)
		}
		err = e.resolve(open, Resolution{Answer: answer, Note: text, By: BySystem})
	case state != "" || len(vars) > 0:
		if state != "" {
			_, w, lerr := LoadWorkflow(e.Home, r.Kekkai, r.Workflow)
			if lerr != nil {
				return nil, lerr
			}
			if w.States[state] != nil && !w.ReachableWithoutHuman(r.State, state) {
				return nil, uerr("FORBIDDEN", "run %s cannot get from %s to %s without a human-only step: only the owner can move it", r.ID, r.State, state)
			}
		}
		err = e.upgrade(r, state, vars)
	case r.Step != nil && r.Step.Kind == kamui.KWait && r.Step.Phase == "waiting" && !r.Step.probing && e.def(r).States[r.State].Step.Probe != "":
		t := now()
		r.Step.probeAt = &t
		e.scheduleRun(r)
	}
	if err != nil {
		return nil, err
	}
	e.event(r, "system.instruction", map[string]any{"text": text, "state": state, "answer": answer, "vars": vars})
	e.event(r, "run.say", map[string]any{"state": r.State, "text": "🛠 system: " + text})
	return r, nil
}

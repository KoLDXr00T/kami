package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"time"

	"kami/internal/kamui"
)

// Pause holds a kekkai's runs: a run entering a state parks there instead (step kind
// "paused"), probes stop, cron triggers skip their fires (none are caught up on resume). Running workers and scripts finish; open
// contracts stay answerable and their answer parks the next state. Workflows in Except keep
// going (e.g. the owner's chats). Until, when set, resumes it by itself.
type Pause struct {
	Until  *time.Time `json:"until,omitempty"`
	Except []string   `json:"except,omitempty"`
	Since  time.Time  `json:"since"`
}

// ponytail: one JSON file; pauses are rare and tiny.
func (e *Engine) pausesFile() string { return filepath.Join(e.Home, "pauses.json") }

func (e *Engine) loadPauses() {
	raw, err := os.ReadFile(e.pausesFile())
	if err == nil {
		json.Unmarshal(raw, &e.pauses)
	}
	if e.pauses == nil {
		e.pauses = map[string]*Pause{}
	}
	for k, p := range e.pauses {
		e.armResume(k, p)
	}
}

func (e *Engine) savePauses() error {
	raw, _ := json.MarshalIndent(e.pauses, "", "  ")
	return os.WriteFile(e.pausesFile(), raw, 0o600)
}

func (e *Engine) held(r *Run) bool {
	p := e.pauses[r.Kekkai]
	return p != nil && !slices.Contains(p.Except, r.Workflow)
}

func (e *Engine) armResume(k string, p *Pause) {
	if p.Until != nil {
		e.setTimer("pause:"+k, *p.Until, func() { e.resume(k) })
	}
}

// PauseKekkai holds kekkai k (see Pause).
func (e *Engine) PauseKekkai(k string, until *time.Time, except []string) (*Pause, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if until != nil && !until.After(now()) {
		return nil, uerr("INVALID", "--until %s is in the past", until.Local().Format(time.RFC3339))
	}
	p := &Pause{Until: until, Except: except, Since: now()}
	e.pauses[k] = p
	if err := e.savePauses(); err != nil {
		return nil, err
	}
	e.stopTimer("pause:" + k)
	e.armResume(k, p)
	e.event(nil, "kekkai.paused", map[string]any{"kekkai": k, "until": until, "except": except})
	return p, nil
}

// ResumeKekkai lifts k's pause now.
func (e *Engine) ResumeKekkai(k string) ([]string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.pauses[k] == nil {
		return nil, uerr("CONFLICT", "kekkai %s is not paused", k)
	}
	return e.resume(k), nil
}

func (e *Engine) resume(k string) []string {
	p := e.pauses[k]
	paused := now().Sub(p.Since)
	delete(e.pauses, k)
	e.stopTimer("pause:" + k)
	e.savePauses()
	var ids []string
	for id, r := range e.runs {
		if r.Kekkai == k && !Terminal(r.Status) {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	var resumed []string
	for _, id := range ids {
		r := e.runs[id]
		switch {
		case slices.Contains(p.Except, r.Workflow): // it kept going: nothing to make up
		case r.Step != nil && r.Step.Kind == "paused":
			resumed = append(resumed, id)
			e.enter(r, r.State)
		case r.Step != nil && r.Step.Kind == kamui.KWait && r.Step.Phase == "waiting" && !r.Step.probing:
			// the pause did not count: its timeout and timer move on by its length
			for _, t := range []*time.Time{r.Step.Deadline, r.Step.WakeAt} {
				if t != nil {
					*t = t.Add(paused)
				}
			}
			r.Step.probeAt = nil // probe again now, not at the time the pause pushed it to
			e.saveRun(r, "resumed")
			e.scheduleRun(r)
		}
	}
	e.event(nil, "kekkai.resumed", map[string]any{"kekkai": k, "runs": resumed})
	return resumed
}

// park holds r at state name until its kekkai resumes; enter runs it then.
func (e *Engine) park(r *Run, name string) {
	r.State, r.Step, r.Status = name, &StepState{Kind: "paused", Phase: "waiting"}, StWaiting
	e.saveRun(r, "paused")
	e.event(r, "run.paused", map[string]any{"state": name})
}

// probeHeld pushes a held wait's next check to the end of the pause (or a minute out).
func (e *Engine) probeHeld(r *Run) {
	t := now().Add(time.Minute)
	if p := e.pauses[r.Kekkai]; p != nil && p.Until != nil {
		t = *p.Until
	}
	r.Step.probeAt = &t
}

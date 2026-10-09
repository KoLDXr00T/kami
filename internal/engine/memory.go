package engine

import (
	"strings"

	"kami/internal/state"
)

// A run's conversation, from its kami_* vars (set by inputs and trusted
// scripts only: a worker may not report kami_* vars). A run without
// kami_conv is the owner's.
type conv struct{ kind, peer, channel string } // kind: owner | dm | group

func runConv(r *Run) conv {
	s := func(k string) string { v, _ := r.Vars[k].(string); return v }
	c := conv{kind: s("kami_conv"), peer: s("kami_peer"), channel: s("kami_channel")}
	if c.kind == "" {
		c.kind = "owner"
	}
	return c
}

// readable is the scopes this conversation may hear; nil means all.
func (c conv) readable() []string {
	switch c.kind {
	case "dm":
		return []string{"public", "dm:" + c.peer}
	case "group":
		return []string{"public", "chan:" + c.channel}
	}
	return nil
}

// writeScope is where a fact said in this conversation goes: the owner picks
// (private by default); a DM or a channel only ever writes to itself.
func (c conv) writeScope(asked string) (string, error) {
	switch c.kind {
	case "dm":
		return "dm:" + c.peer, nil
	case "group":
		return "chan:" + c.channel, nil
	}
	if asked == "" {
		return "private", nil
	}
	if asked == "public" || asked == "private" || strings.HasPrefix(asked, "dm:") || strings.HasPrefix(asked, "chan:") {
		return asked, nil
	}
	return "", uerr("INVALID", "scope %q: want public, private, dm:<user> or chan:<channel>", asked)
}

// callerConv is the conversation of the caller: a worker's run (its token
// must be live), or the owner for a call without one.
func (e *Engine) callerConv(runID, tok string) (conv, string, error) {
	if tok == "" {
		return conv{kind: "owner"}, "owner", nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	r, err := e.workerRun(runID, tok)
	if err != nil {
		return conv{}, "", err
	}
	return runConv(r), "run " + r.ID, nil
}

// Remember saves a fact in the scope the caller's conversation allows.
func (e *Engine) Remember(runID, tok, about, fact, scope string) (*state.Memory, error) {
	c, source, err := e.callerConv(runID, tok)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(fact) == "" {
		return nil, uerr("INVALID", "a fact is required")
	}
	if scope, err = c.writeScope(scope); err != nil {
		return nil, err
	}
	m := state.Memory{ID: NewID(), About: strings.TrimSpace(about), Fact: strings.TrimSpace(fact), Scope: scope,
		Source: source, Created: now().Format("2006-01-02T15:04:05Z")}
	if err := e.Mem.SaveMemory(m); err != nil {
		return nil, err
	}
	return &m, nil
}

// Recall returns the facts the caller's conversation may hear.
func (e *Engine) Recall(runID, tok, about, text string) ([]state.Memory, error) {
	c, _, err := e.callerConv(runID, tok)
	if err != nil {
		return nil, err
	}
	return e.Mem.Memories(c.readable(), about, text, 100)
}

// Forget deletes a fact; only the owner (no worker token) may.
func (e *Engine) Forget(tok, id string) error {
	if tok != "" {
		return uerr("FORBIDDEN", "only the owner can make POWER forget; ask the owner")
	}
	return e.Mem.ForgetMemory(id)
}

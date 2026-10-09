package engine

import (
	"strings"

	"kami/internal/state"
)

// A run's conversation, from its kami_* inputs (no worker, signal, script or
// answer can set kami_* vars). kind: cli (the owner at the terminal), owner
// (his chat), dm, group; a run with no kami_conv (goal, ticket, ...) has no
// memory at all.
type conv struct{ kind, peer, channel string }

func runConv(r *Run) conv {
	s := func(k string) string { v, _ := r.Vars[k].(string); return v }
	return conv{kind: s("kami_conv"), peer: s("kami_peer"), channel: s("kami_channel")}
}

// readable is the scopes this conversation may hear; nil means all. The
// owner's chat does not hear what coworkers told POWER (dm:, chan:): that text
// is theirs, and it must never reach a worker with the owner's tools.
func (c conv) readable() ([]string, error) {
	switch c.kind {
	case "cli":
		return nil, nil
	case "owner":
		return []string{"public", "private"}, nil
	case "dm":
		return []string{"public", "dm:" + c.peer}, nil
	case "group":
		return []string{"public", "chan:" + c.channel}, nil
	}
	return nil, uerr("FORBIDDEN", "this run has no memory (only conversations do)")
}

// writeScope is where a fact said in this conversation goes: the owner picks
// (private by default; his chat only public or private); a DM or a channel
// only ever writes to itself.
func (c conv) writeScope(asked string) (string, error) {
	switch c.kind {
	case "dm":
		return "dm:" + c.peer, nil
	case "group":
		return "chan:" + c.channel, nil
	case "owner", "cli":
		if asked == "" {
			return "private", nil
		}
		if asked == "public" || asked == "private" ||
			c.kind == "cli" && (strings.HasPrefix(asked, "dm:") || strings.HasPrefix(asked, "chan:")) {
			return asked, nil
		}
		return "", uerr("INVALID", "scope %q: want public or private", asked)
	}
	return "", uerr("FORBIDDEN", "this run has no memory (only conversations do)")
}

// callerConv is the conversation of the caller: a worker's run (its token
// must be live), or the owner at the terminal for a call without one.
// ponytail: "no token" is trusted as the owner. A worker that can run any
// shell command (no permission_mode) can drop its token, and also read
// memento directly; memory scopes hold for permission-restricted workers.
func (e *Engine) callerConv(runID, tok string) (conv, string, error) {
	if tok == "" {
		return conv{kind: "cli"}, "owner", nil
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
	scopes, err := c.readable()
	if err != nil {
		return nil, err
	}
	return e.Mem.Memories(scopes, about, text, 100)
}

// Forget deletes a fact; only the owner (no worker token) may.
func (e *Engine) Forget(tok, id string) error {
	if tok != "" {
		return uerr("FORBIDDEN", "only the owner can make POWER forget; ask the owner")
	}
	return e.Mem.ForgetMemory(id)
}

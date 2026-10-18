// Package state persists Kami's durable records (runs, contracts, Aura
// snapshots, pinned workflow bundles) in a memento namespace. Every write is
// one `memento db exec`, which memento turns into one git commit.
package state

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Memento is a client for one memento namespace.
type Memento struct {
	NS  string
	Bin string   // memento binary; "memento" on PATH by default
	Env []string // process env for memento (MEMENTO_STORE etc.); nil inherits
}

type envelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Error is a memento command failure.
type Error struct {
	Code, Message string
}

func (e *Error) Error() string { return "memento: " + e.Code + ": " + e.Message }

// timeout bounds one memento call; a hung git or store lock surfaces as an error.
const timeout = time.Minute

func (m *Memento) run(args ...string) (json.RawMessage, error) {
	bin := m.Bin
	if bin == "" {
		bin = "memento"
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, append([]string{"--json"}, args...)...)
	if m.Env != nil {
		cmd.Env = m.Env
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	runErr := cmd.Run()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("memento %s: timed out after %v", args[0], timeout)
	}
	var env envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		if runErr != nil {
			return nil, fmt.Errorf("memento %s: %v: %s", args[0], runErr, strings.TrimSpace(errb.String()))
		}
		return nil, fmt.Errorf("memento %s: unreadable output: %q", args[0], out.String())
	}
	if !env.OK {
		if env.Error != nil {
			return nil, &Error{Code: env.Error.Code, Message: env.Error.Message}
		}
		return nil, fmt.Errorf("memento %s failed: %s", args[0], strings.TrimSpace(errb.String()))
	}
	return env.Data, nil
}

// Exec runs one write statement (or script); msg becomes the commit message.
func (m *Memento) Exec(sql, msg string) error {
	_, err := m.run("db", "exec", m.NS, sql, "-m", msg)
	return err
}

// Query runs one SELECT and returns rows keyed by column.
func (m *Memento) Query(sql string) ([]map[string]any, error) {
	raw, err := m.run("db", "query", m.NS, sql)
	if err != nil {
		return nil, err
	}
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("memento query: %w", err)
	}
	return rows, nil
}

const schema = `
CREATE TABLE IF NOT EXISTS runs(id TEXT PRIMARY KEY, kekkai TEXT NOT NULL, workflow TEXT NOT NULL,
  status TEXT NOT NULL, parent TEXT, updated_at TEXT NOT NULL, doc TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS contracts(id TEXT PRIMARY KEY, run_id TEXT NOT NULL, kekkai TEXT NOT NULL,
  status TEXT NOT NULL, updated_at TEXT NOT NULL, doc TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS auras(run_id TEXT NOT NULL, step TEXT NOT NULL, seq INTEGER NOT NULL,
  created_at TEXT NOT NULL, doc TEXT NOT NULL, PRIMARY KEY(run_id, step, seq));
CREATE TABLE IF NOT EXISTS defs(hash TEXT PRIMARY KEY, kekkai TEXT NOT NULL, workflow TEXT NOT NULL,
  entry TEXT NOT NULL, files TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS triggers(id TEXT PRIMARY KEY, last_fired TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS memory(id TEXT PRIMARY KEY, about TEXT NOT NULL, fact TEXT NOT NULL,
  scope TEXT NOT NULL, source TEXT NOT NULL, created_at TEXT NOT NULL);
`

// Ensure creates the namespace and tables if they do not exist yet.
func (m *Memento) Ensure() error {
	if _, err := m.run("ns", "create", m.NS, "-d", "kami: runs, contracts, aura snapshots"); err != nil {
		if e, ok := err.(*Error); !ok || e.Code != "NS_EXISTS" {
			return err
		}
	}
	rows, err := m.Query("SELECT count(*) AS n FROM sqlite_master WHERE type='table' AND name IN ('runs','contracts','auras','defs','triggers','memory')")
	if err == nil && len(rows) == 1 {
		if n, _ := rows[0]["n"].(float64); n == 6 {
			return nil
		}
	}
	return m.Exec(schema, "kami: schema")
}

// Q quotes a string as an SQL literal, hex-encoded. memento db exec takes its
// SQL only as an argument, so every value lands in memento's argv; in plain
// text a `pkill -f "<cmd>"` run by a worker matches the run doc that quotes
// that command and kills memento mid-write.
func Q(s string) string {
	return "CAST(X'" + hex.EncodeToString([]byte(strings.ReplaceAll(s, "\x00", ""))) + "' AS TEXT)"
}

func qn(s string) string {
	if s == "" {
		return "NULL"
	}
	return Q(s)
}

func doc(v any) (string, error) {
	raw, err := json.Marshal(v)
	return string(raw), err
}

// SaveRun upserts a run.
func (m *Memento) SaveRun(id, kekkai, workflow, status, parent, updated string, v any, msg string) error {
	d, err := doc(v)
	if err != nil {
		return err
	}
	return m.Exec(fmt.Sprintf(`INSERT INTO runs(id,kekkai,workflow,status,parent,updated_at,doc) VALUES(%s,%s,%s,%s,%s,%s,%s)
ON CONFLICT(id) DO UPDATE SET status=excluded.status, updated_at=excluded.updated_at, doc=excluded.doc;`,
		Q(id), Q(kekkai), Q(workflow), Q(status), qn(parent), Q(updated), Q(d)), msg)
}

// SaveContract upserts a contract.
func (m *Memento) SaveContract(id, runID, kekkai, status, updated string, v any, msg string) error {
	d, err := doc(v)
	if err != nil {
		return err
	}
	return m.Exec(fmt.Sprintf(`INSERT INTO contracts(id,run_id,kekkai,status,updated_at,doc) VALUES(%s,%s,%s,%s,%s,%s)
ON CONFLICT(id) DO UPDATE SET status=excluded.status, updated_at=excluded.updated_at, doc=excluded.doc;`,
		Q(id), Q(runID), Q(kekkai), Q(status), Q(updated), Q(d)), msg)
}

// AddAura appends an Aura snapshot.
func (m *Memento) AddAura(runID, step string, seq int, created string, v any) error {
	d, err := doc(v)
	if err != nil {
		return err
	}
	return m.Exec(fmt.Sprintf(`INSERT INTO auras(run_id,step,seq,created_at,doc) VALUES(%s,%s,%d,%s,%s);`,
		Q(runID), Q(step), seq, Q(created), Q(d)), fmt.Sprintf("kami: aura %s/%s #%d", runID, step, seq))
}

// SaveDef stores a pinned workflow bundle once per hash.
func (m *Memento) SaveDef(hash, kekkai, workflow, entry string, files map[string]string) error {
	d, err := doc(files)
	if err != nil {
		return err
	}
	return m.Exec(fmt.Sprintf(`INSERT OR IGNORE INTO defs(hash,kekkai,workflow,entry,files) VALUES(%s,%s,%s,%s,%s);`,
		Q(hash), Q(kekkai), Q(workflow), Q(entry), Q(d)), fmt.Sprintf("kami: pin %s/%s %s", kekkai, workflow, hash[:12]))
}

// SaveTrigger records when a cron trigger last fired.
func (m *Memento) SaveTrigger(id, lastFired string) error {
	return m.Exec(fmt.Sprintf(`INSERT INTO triggers(id,last_fired) VALUES(%s,%s)
ON CONFLICT(id) DO UPDATE SET last_fired=excluded.last_fired;`, Q(id), Q(lastFired)), "kami: trigger "+id)
}

// Docs reads the doc column of every row in table matching where (SQL).
func (m *Memento) Docs(table, where string) ([]json.RawMessage, error) {
	q := "SELECT doc FROM " + table
	if where != "" {
		q += " WHERE " + where
	}
	rows, err := m.Query(q)
	if err != nil {
		return nil, err
	}
	out := make([]json.RawMessage, 0, len(rows))
	for _, r := range rows {
		s, _ := r["doc"].(string)
		out = append(out, json.RawMessage(s))
	}
	return out, nil
}

// Memory is one remembered fact. Scope decides who may hear it: public,
// private (the owner only), dm:<user> (that DM and the owner), chan:<channel>
// (that channel and the owner).
type Memory struct {
	ID      string `json:"id"`
	About   string `json:"about"`
	Fact    string `json:"fact"`
	Scope   string `json:"scope"`
	Source  string `json:"source"`
	Created string `json:"created_at"`
}

// SaveMemory adds a fact.
func (m *Memento) SaveMemory(f Memory) error {
	return m.Exec(fmt.Sprintf(`INSERT INTO memory(id,about,fact,scope,source,created_at) VALUES(%s,%s,%s,%s,%s,%s);`,
		Q(f.ID), Q(f.About), Q(f.Fact), Q(f.Scope), Q(f.Source), Q(f.Created)), "kami: remember "+f.ID)
}

// Memories returns the facts in scopes (all scopes when nil) whose about or
// fact contains each of the needles, newest first, at most limit.
func (m *Memento) Memories(scopes []string, about, text string, limit int) ([]Memory, error) {
	var where []string
	if scopes != nil {
		qs := make([]string, len(scopes))
		for i, s := range scopes {
			qs[i] = Q(s)
		}
		where = append(where, "scope IN ("+strings.Join(append(qs, "''"), ",")+")")
	}
	if about != "" {
		where = append(where, "lower(about) LIKE "+Q("%"+strings.ToLower(about)+"%"))
	}
	if text != "" {
		where = append(where, "(lower(fact) LIKE "+Q("%"+strings.ToLower(text)+"%")+" OR lower(about) LIKE "+Q("%"+strings.ToLower(text)+"%")+")")
	}
	q := "SELECT id,about,fact,scope,source,created_at FROM memory"
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	rows, err := m.Query(q + fmt.Sprintf(" ORDER BY created_at DESC LIMIT %d", limit))
	if err != nil {
		return nil, err
	}
	out := make([]Memory, 0, len(rows))
	for _, r := range rows {
		s := func(k string) string { v, _ := r[k].(string); return v }
		out = append(out, Memory{ID: s("id"), About: s("about"), Fact: s("fact"), Scope: s("scope"), Source: s("source"), Created: s("created_at")})
	}
	return out, nil
}

// ForgetMemory deletes a fact.
func (m *Memento) ForgetMemory(id string) error {
	return m.Exec("DELETE FROM memory WHERE id="+Q(id)+";", "kami: forget "+id)
}

// Def is a pinned workflow bundle.
type Def struct {
	Hash, Kekkai, Workflow, Entry string
	Files                         map[string]string
}

// Defs reads every pinned bundle.
func (m *Memento) Defs() ([]Def, error) {
	rows, err := m.Query("SELECT hash, kekkai, workflow, entry, files FROM defs")
	if err != nil {
		return nil, err
	}
	var out []Def
	for _, r := range rows {
		d := Def{}
		d.Hash, _ = r["hash"].(string)
		d.Kekkai, _ = r["kekkai"].(string)
		d.Workflow, _ = r["workflow"].(string)
		d.Entry, _ = r["entry"].(string)
		fs, _ := r["files"].(string)
		if err := json.Unmarshal([]byte(fs), &d.Files); err != nil {
			return nil, fmt.Errorf("def %s: %w", d.Hash, err)
		}
		out = append(out, d)
	}
	return out, nil
}

// Triggers reads trigger id -> last fired.
func (m *Memento) Triggers() (map[string]string, error) {
	rows, err := m.Query("SELECT id, last_fired FROM triggers")
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, r := range rows {
		id, _ := r["id"].(string)
		lf, _ := r["last_fired"].(string)
		out[id] = lf
	}
	return out, nil
}

// TestStore makes an empty memento store in dir (git init) and returns the env
// that points memento at it. It never touches the user's memento config.
func TestStore(dir string) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	cmd := exec.Command("git", "init", "-q", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("git init: %v: %s", err, out)
	}
	env := []string{}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "MEMENTO_STORE=") && !strings.HasPrefix(kv, "MEMENTO_REMOTE=") {
			env = append(env, kv)
		}
	}
	return append(env, "MEMENTO_STORE="+dir), nil
}

package state

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"testing"
	"time"
)

func store(t *testing.T) *Memento {
	t.Helper()
	if _, err := exec.LookPath("memento"); err != nil {
		t.Skip("memento not on PATH")
	}
	env, err := TestStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := &Memento{NS: "kami", Env: env}
	if err := m.Ensure(); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestRoundTrip(t *testing.T) {
	m := store(t)
	if err := m.Ensure(); err != nil { // idempotent
		t.Fatal(err)
	}
	type doc struct {
		ID   string `json:"id"`
		Note string `json:"note"`
	}
	if err := m.SaveRun("r1", "test", "wf", "running", "", "t0", doc{"r1", "it's quoted"}, "kami: run r1"); err != nil {
		t.Fatal(err)
	}
	if err := m.SaveRun("r1", "test", "wf", "done", "", "t1", doc{"r1", "second"}, "kami: run r1"); err != nil {
		t.Fatal(err)
	}
	docs, err := m.Docs("runs", "status = 'done'")
	if err != nil || len(docs) != 1 {
		t.Fatalf("docs = %v, %v", docs, err)
	}
	var d doc
	json.Unmarshal(docs[0], &d)
	if d.Note != "second" {
		t.Fatalf("upsert lost: %+v", d)
	}
	if err := m.SaveDef("abcdef0123456789", "test", "wf", "workflows/wf.star", map[string]string{"workflows/wf.star": "x = 'y'"}); err != nil {
		t.Fatal(err)
	}
	defs, err := m.Defs()
	if err != nil || len(defs) != 1 || defs[0].Files["workflows/wf.star"] != "x = 'y'" {
		t.Fatalf("defs = %+v, %v", defs, err)
	}
	if err := m.AddAura("r1", "s", 1, "t", map[string]any{"goal": "g"}); err != nil {
		t.Fatal(err)
	}
}

// TestWriteLatency is the M1 benchmark: 200 contract writes through memento.
// KAMI_BENCH=1 go test ./internal/state -run WriteLatency -v
func TestWriteLatency(t *testing.T) {
	if os.Getenv("KAMI_BENCH") == "" {
		t.Skip("set KAMI_BENCH=1")
	}
	m := store(t)
	var lat []time.Duration
	body := map[string]any{"title": "Approve the draft?", "body": string(make([]byte, 0)), "options": []string{"approve", "reject"}}
	for i := 0; i < 200; i++ {
		id := fmt.Sprintf("c%03d", i)
		start := time.Now()
		if err := m.SaveContract(id, "r1", "test", "open", time.Now().UTC().Format(time.RFC3339), body, "kami: contract "+id); err != nil {
			t.Fatal(err)
		}
		lat = append(lat, time.Since(start))
	}
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	t.Logf("200 writes: p50 %v  p95 %v  max %v", lat[100], lat[190], lat[199])
}

// A worker's `pkill -f "<cmd>"` matched memento's argv when the run doc rode in
// it as plain text; values must not appear verbatim on memento's command line.
func TestPayloadNotInArgv(t *testing.T) {
	dir := t.TempDir()
	bin := dir + "/memento"
	os.WriteFile(bin, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \""+dir+"/argv\"\necho '{\"ok\":true,\"data\":null}'\n"), 0o755)
	m := &Memento{NS: "kami", Bin: bin}
	payload := `pkill -f "sleep 4242"`
	if err := m.SaveRun("r1", "test", "wf", "running", "", "t0", map[string]any{"cmd": payload}, "kami: run r1"); err != nil {
		t.Fatal(err)
	}
	argv, _ := os.ReadFile(dir + "/argv")
	if len(argv) == 0 || bytes.Contains(argv, []byte("sleep 4242")) {
		t.Fatalf("payload in memento argv:\n%s", argv)
	}
}

// A memento without stdin support parses "-" as SQL; kami must say to upgrade.
func TestOldMementoSaysUpgrade(t *testing.T) {
	bin := t.TempDir() + "/memento"
	os.WriteFile(bin, []byte(`#!/bin/sh
echo '{"ok":false,"data":null,"error":{"code":"SQL_ERROR","message":"SQL logic error: near \"-\": syntax error (1)"}}'
exit 6
`), 0o755)
	m := &Memento{NS: "kami", Bin: bin}
	if err := m.Exec("SELECT 1", "x"); err == nil || !strings.Contains(err.Error(), "upgrade memento") {
		t.Fatalf("err = %v", err)
	}
}

VERSION    := $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
LDFLAGS    := -ldflags "-X kami/internal/engine.Version=$(VERSION)"
BIN_DIR    := $(HOME)/.local/bin
SKILLS_DIR := $(HOME)/.claude/skills
AGENTS     := $(HOME)/Library/LaunchAgents

.PHONY: build adapters test race smoke bench vet install install-skills launchd-install launchd-uninstall clean

build:
	go build $(LDFLAGS) -o bin/kami ./cmd/kami

# Kami's Slack surface (contracts as owner DMs, canvases, dm)
adapters:
	go build -o bin/kami-slack ./adapters/slack

vet:
	go vet ./...

# unit tests + the e2e suite (fake workers, temp homes, temp memento stores)
test: vet
	go test ./...

# the same, with the daemon built and run under the race detector
race:
	KAMI_E2E_RACE=1 go test -race -count=1 ./...

# real Claude workers: blocker → sleep → wake, and session resume. Uses tokens.
smoke:
	KAMI_SMOKE=1 go test -count=1 -run Smoke -v ./e2e

# memento write latency, 200 contract writes
bench:
	KAMI_BENCH=1 go test -count=1 -run WriteLatency -v ./internal/state

# Copy then rename: overwriting a binary launchd is executing leaves a
# half-mapped image and the restart dies.
install: build adapters install-skills
	@mkdir -p $(BIN_DIR)
	cp bin/kami $(BIN_DIR)/kami.tmp && mv -f $(BIN_DIR)/kami.tmp $(BIN_DIR)/kami
	cp bin/kami-slack $(BIN_DIR)/kami-slack.tmp && mv -f $(BIN_DIR)/kami-slack.tmp $(BIN_DIR)/kami-slack
	@echo "installed $(BIN_DIR)/kami $(BIN_DIR)/kami-slack"

# The repo is the source of truth for the skills; the install is a symlink.
install-skills:
	@mkdir -p $(SKILLS_DIR)
	@for s in kami kami-author kami-worker; do \
	  rm -rf "$(SKILLS_DIR)/$$s"; \
	  ln -sfn "$(CURDIR)/skills/$$s" "$(SKILLS_DIR)/$$s"; \
	  echo "linked $(SKILLS_DIR)/$$s -> skills/$$s"; \
	done

# Runs `kami daemon` and `kami-slack serve` at login and restarts them if they
# die. launchd does not read your shell profile, so PATH is spelled out
# (memento, claude, codex).
launchd-install: install
	@mkdir -p $(AGENTS) $(HOME)/.kami
	@for a in daemon slack; do \
	  sed -e "s#__HOME__#$(HOME)#g" -e "s#__PATH__#$(BIN_DIR):/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin#g" \
	    deploy/ai.kami.$$a.plist > $(AGENTS)/ai.kami.$$a.plist; \
	  launchctl bootout gui/$$(id -u) $(AGENTS)/ai.kami.$$a.plist 2>/dev/null; \
	  launchctl bootstrap gui/$$(id -u) $(AGENTS)/ai.kami.$$a.plist && echo "loaded ai.kami.$$a"; \
	done
	@echo "logs: $(HOME)/.kami/daemon.log, $(HOME)/.kami/kami-slack.log"

launchd-uninstall:
	-@for a in daemon slack; do launchctl bootout gui/$$(id -u) $(AGENTS)/ai.kami.$$a.plist; rm -f $(AGENTS)/ai.kami.$$a.plist; done

clean:
	rm -rf bin

# Shared Multica fragments for the acme Kekkai.

def preflight(then):
    """Check the Multica profile, glab auth and that the local daemon is stopped."""
    state("preflight", script("scripts/preflight"),
          ok = then, auth = "proxy_down")
    state("proxy_down", blocker("Multica 401: the CF Access proxy is down",
                                resolver = "resolvers/acme-init", mode = "afk"),
          fixed = "preflight")

def enqueue(then, on_dead):
    """Prove a builder run was enqueued for {ticket_id}.

    Rerun only when the assign enqueued nothing. An expired builder login goes to
    ccpush without asking; if ccpush fails too, the run goes to `on_dead`.
    """
    state("await_run", script("scripts/await-run {ticket_id}", timeout = "4m"),
          enqueued = then, none = "rerun", auth_failed = "ccpush")
    state("rerun", script("scripts/rerun {ticket_id}", max_visits = 2),
          ok = "await_run")
    state("ccpush", blocker("The builder's Claude login expired on the Multica VM",
                            resolver = "resolvers/ccpush", mode = "afk"),
          fixed = "rerun", failed = on_dead)

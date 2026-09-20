#!/bin/sh
# docker-entrypoint.sh — runtime entrypoint for the ezHealthKonnect app container.
#
# Runs the Go backend (go-api) under a crash-restart watchdog and the Node.js
# frontend (server.js) as siblings, and forwards SIGTERM/SIGINT to BOTH on
# container stop so main.go's own graceful-shutdown path (drains connectors
# via processingEngine.Stop(), writes an APPLICATION_STOPPED audit row,
# drains in-flight HTTP requests) actually runs instead of being skipped in
# favor of Docker's SIGKILL.
#
# WHY THIS EXISTS: the previous approach — plain `sh -c "./go-api & ... &&
# node server.js"`, inlined separately (and inconsistently) in both this
# file's Dockerfile CMD and docker-compose.yml's command: block — does NOT
# forward a received SIGTERM to backgrounded children; plain sh only does
# that with an explicit trap. Confirmed directly: `docker-compose restart`/
# `stop` produced zero APPLICATION_STOPPED audit_logs rows across repeated
# attempts, while sending SIGTERM to the go-api process directly fired
# main.go's handler in under a second. The gap was in signal DELIVERY, not
# in main.go's own (already-correct) signal.Notify handler.
#
# go-api's own PID is tracked via a PID FILE, not a shell variable, because
# it's started inside a backgrounded subshell (the watchdog loop below) —
# POSIX shell subshells get their own copy of variables at fork time, so a
# variable set inside `( ... ) &` is never visible back in this parent
# shell, which is where the trap handler below actually runs.
#
# TIMING: main.go's graceful shutdown can legitimately take up to ~45s in
# the worst case (processingEngine.Stop()'s 30s in-flight-message drain,
# PIPELINE_SHUTDOWN_TIMEOUT_SECONDS, plus a further 15s HTTP drain) — this
# script polls (see term_handler below) after forwarding the signal so
# that finishes rather than exiting early. docker-compose.yml sets a
# matching stop_grace_period so Docker doesn't SIGKILL the container
# before that sequence completes.
#
# A SECOND, SEPARATE bug was found fixing THIS script: even with delivery
# and waiting both correct, a freshly-(re)started go-api process could still
# silently skip its own shutdown sequence if a SIGTERM arrived while it was
# still early in its own startup — main.go used to call signal.Notify quite
# late (right before starting the HTTP server), so SIGTERM had its OS-default
# (immediate-kill) disposition until then. Fixed in main.go itself by moving
# signal.Notify to the very first lines of func main() — see that file's own
# comment there for the full story (confirmed via repeated real restarts).

set -u

GOAPI_PIDFILE="/tmp/goapi.pid"
SHUTDOWN_FLAG="/tmp/.shutting_down"
NODE_PID=""

rm -f "$GOAPI_PIDFILE" "$SHUTDOWN_FLAG"

term_handler() {
    echo "🛑 Received termination signal — shutting down gracefully..."
    touch "$SHUTDOWN_FLAG"

    GOAPI_PID=""
    if [ -f "$GOAPI_PIDFILE" ]; then
        GOAPI_PID="$(cat "$GOAPI_PIDFILE" 2>/dev/null || true)"
    fi

    if [ -n "$GOAPI_PID" ] && kill -0 "$GOAPI_PID" 2>/dev/null; then
        echo "  → forwarding SIGTERM to go-api (pid $GOAPI_PID) — waiting for its own graceful shutdown (up to ~55s)..."
        kill -TERM "$GOAPI_PID" 2>/dev/null || true
    fi

    if [ -n "$NODE_PID" ] && kill -0 "$NODE_PID" 2>/dev/null; then
        echo "  → forwarding SIGTERM to node server.js (pid $NODE_PID)"
        kill -TERM "$NODE_PID" 2>/dev/null || true
    fi

    # Poll for both processes to actually exit, rather than a bare `wait`.
    # CONFIRMED BY DIRECT TESTING to be necessary, not just defensive: an
    # earlier version of this script used a bare `wait` here and it
    # returned in ~15ms — before go-api (started inside the backgrounded
    # watchdog subshell below, so it's a GRANDCHILD of this shell, not a
    # direct job `wait` tracks) had done anything at all. go-api kept
    # running, completely unaffected, for minutes afterward while this
    # script had already exited and Docker believed shutdown was complete.
    # Polling via kill -0 sidesteps that shell/job-control ambiguity
    # entirely — no dependency on how a specific `sh` implementation
    # tracks background jobs across a trap-handler/subshell boundary.
    elapsed=0
    max_wait=55
    while [ "$elapsed" -lt "$max_wait" ]; do
        goapi_alive=0
        node_alive=0
        [ -n "$GOAPI_PID" ] && kill -0 "$GOAPI_PID" 2>/dev/null && goapi_alive=1
        [ -n "$NODE_PID" ] && kill -0 "$NODE_PID" 2>/dev/null && node_alive=1
        if [ "$goapi_alive" -eq 0 ] && [ "$node_alive" -eq 0 ]; then
            break
        fi
        sleep 1
        elapsed=$((elapsed + 1))
    done

    if [ "$elapsed" -ge "$max_wait" ]; then
        echo "⚠️  Timed out after ${max_wait}s waiting for graceful shutdown — exiting anyway."
    fi

    echo "✅ Shutdown complete."
    exit 0
}
trap term_handler TERM INT

# Watchdog loop for go-api: auto-restarts on an unexpected crash (3s
# backoff, matching the previous behavior), but stops restarting once
# SHUTDOWN_FLAG appears (set by term_handler above) rather than racing a
# deliberate shutdown.
(
    while [ ! -f "$SHUTDOWN_FLAG" ]; do
        ./go-api &
        GOAPI_PID=$!
        echo "$GOAPI_PID" > "$GOAPI_PIDFILE"
        wait "$GOAPI_PID"
        EXIT_CODE=$?
        rm -f "$GOAPI_PIDFILE"
        if [ -f "$SHUTDOWN_FLAG" ]; then
            break
        fi
        echo "⚠️  go-api exited (code $EXIT_CODE), restarting in 3s..."
        sleep 3
    done
) &

sleep 3
node server.js &
NODE_PID=$!

wait "$NODE_PID"

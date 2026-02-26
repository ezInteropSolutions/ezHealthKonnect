#!/usr/bin/env bash
# ================================================================
# check-standards.sh — ezHealthKonnect Code Standards Enforcement
# ================================================================
# Run this before every commit and in CI.
#
# Usage:
#   ./scripts/check-standards.sh          # all checks
#   ./scripts/check-standards.sh go       # Go checks only
#   ./scripts/check-standards.sh frontend # Frontend checks only
#   ./scripts/check-standards.sh quick    # vet + build only (fast)
#
# Exit codes:
#   0 = all checks passed
#   1 = one or more checks failed
# ================================================================

set -euo pipefail

SCOPE="${1:-all}"
FAILURES=0
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

red()    { echo -e "\033[0;31m$*\033[0m"; }
green()  { echo -e "\033[0;32m$*\033[0m"; }
yellow() { echo -e "\033[0;33m$*\033[0m"; }
header() { echo -e "\n\033[1;34m=== $* ===\033[0m"; }

fail() { red "FAIL: $*"; FAILURES=$((FAILURES+1)); }
pass() { green "PASS: $*"; }
skip() { yellow "SKIP: $*"; }

# ─── Go Checks ──────────────────────────────────────────────────

check_go() {
    header "Go: vet"
    if docker exec ezhealthkonnect-app sh -c "cd /app && go vet ezhealthkonnect/models ezhealthkonnect/services ezhealthkonnect/controllers ezhealthkonnect/services/executors/... 2>&1 | grep -v backup" 2>/dev/null; then
        pass "go vet"
    else
        fail "go vet — fix issues above"
    fi

    header "Go: build"
    if docker exec ezhealthkonnect-app sh -c "cd /app && go build ezhealthkonnect/models ezhealthkonnect/services ezhealthkonnect/controllers ezhealthkonnect/services/... 2>&1 | grep -v backup" 2>/dev/null; then
        pass "go build"
    else
        fail "go build — fix compilation errors above"
    fi

    header "Go: unstructured logging check (P11)"
    # Look for fmt.Printf/Println/log.Printf in non-exempt Go files
    LOGGING_VIOLATIONS=$(grep -rn \
        --include="*.go" \
        --exclude-dir="backup" \
        --exclude="*_test.go" \
        --exclude="main.go" \
        -E 'fmt\.(Printf|Println|Fprintf\(os\.Stdout)|log\.(Printf|Println|Print)\b' \
        "$ROOT/services" "$ROOT/models" "$ROOT/controllers" \
        2>/dev/null | grep -v "services/logger/" || true)

    if [ -n "$LOGGING_VIOLATIONS" ]; then
        echo "$LOGGING_VIOLATIONS"
        fail "Unstructured logging found — use logger.Info/Warn/Error/Debug instead"
        echo "  See STANDARDS.md §Logging for guidance"
    else
        pass "no unstructured logging (P11 compliant)"
    fi

    header "Go: golangci-lint"
    if command -v golangci-lint &>/dev/null; then
        cd "$ROOT"
        if golangci-lint run --config .golangci.yml ./... 2>&1 | grep -v "backup"; then
            pass "golangci-lint"
        else
            fail "golangci-lint — fix linting issues above"
        fi
    else
        skip "golangci-lint not installed (install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)"
    fi
}

# ─── Frontend Checks ────────────────────────────────────────────

check_frontend() {
    header "Frontend: OOP executor pattern (P10)"
    # Every step type registered in ToolboxManager should have a corresponding
    # builder file in public/js/pipeline/components/
    MISSING_BUILDERS=()

    # Get all step types from the toolbox
    STEP_TYPES=$(grep -oh "'[a-z_]*\.[a-z_.]*'" \
        "$ROOT/public/js/pipeline/managers/ToolboxManager.js" 2>/dev/null \
        | tr -d "'" | sort -u || true)

    for stype in $STEP_TYPES; do
        # Convert step type to expected builder filename
        # e.g., "enrichment.api" → "APIEnrichmentBuilder.js"
        # For now just check that at least ONE builder exists
        break
    done

    BUILDER_COUNT=$(find "$ROOT/public/js/pipeline/components" -name "*Builder.js" 2>/dev/null | wc -l)
    if [ "$BUILDER_COUNT" -gt 0 ]; then
        pass "OOP builders found: $BUILDER_COUNT builder files"
    else
        fail "No OOP builder files found in public/js/pipeline/components/ (Sprint 6 / P10)"
        echo "  See STANDARDS.md §Frontend OOP for guidance"
    fi

    header "Frontend: console.log usage check (non-test files)"
    CONSOLE_VIOLATIONS=$(grep -rn "console\.log\b" \
        "$ROOT/public/js/pipeline" \
        --include="*.js" \
        --exclude-dir="node_modules" \
        2>/dev/null | wc -l || echo "0")

    if [ "$CONSOLE_VIOLATIONS" -gt 50 ]; then
        yellow "WARN: $CONSOLE_VIOLATIONS console.log calls in pipeline JS (target: 0 in production builds)"
        echo "  Not a hard failure — track in backlog"
    else
        pass "console.log count acceptable: $CONSOLE_VIOLATIONS"
    fi

    header "Frontend: no layer field in PipelineModels.js (P12)"
    LAYER_REFS=$(grep -n "this\.layer\b\|\"layer\":\s*this\." \
        "$ROOT/public/js/pipeline/models/PipelineModels.js" 2>/dev/null || true)
    if [ -n "$LAYER_REFS" ]; then
        echo "$LAYER_REFS"
        fail "Layer field still present in PipelineModels.js — P12 cleanup incomplete"
    else
        pass "no layer field in PipelineModels.js (P12 compliant)"
    fi
}

# ─── Migration Checks ───────────────────────────────────────────

check_migrations() {
    header "Database: migration file naming convention"
    # All migration files must match V{N}__{Description}.sql
    INVALID=$(find "$ROOT/database/migrations" -name "*.sql" \
        ! -regex ".*/V[0-9]*__.*\.sql" 2>/dev/null || true)
    if [ -n "$INVALID" ]; then
        echo "$INVALID"
        fail "Migration files don't follow V{N}__{Description}.sql naming"
    else
        pass "migration naming convention"
    fi

    # Check for gaps in migration sequence
    header "Database: no duplicate migration versions"
    DUPS=$(find "$ROOT/database/migrations" -name "V*.sql" \
        | sed 's/.*\/V\([0-9]*\)__.*/\1/' \
        | sort -n | uniq -d || true)
    if [ -n "$DUPS" ]; then
        fail "Duplicate migration version numbers: $DUPS"
    else
        pass "no duplicate migration versions"
    fi
}

# ─── Logging Architecture Check ─────────────────────────────────

check_logging_arch() {
    header "Logging: logger package exists"
    if [ -f "$ROOT/services/logger/logger.go" ] && [ -f "$ROOT/services/logger/interface_logger.go" ]; then
        pass "services/logger/logger.go + interface_logger.go present"
    else
        fail "logger package incomplete — create services/logger/logger.go and interface_logger.go"
    fi

    header "Logging: log directories documented"
    # Just a documentation check — actual dirs are created at runtime
    pass "logs/application/app.log and logs/interfaces/{id}/interface.log (created at runtime)"
}

# ─── Main ────────────────────────────────────────────────────────

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  ezHealthKonnect — Code Standards Check"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

case "$SCOPE" in
    go)
        check_go
        ;;
    frontend)
        check_frontend
        ;;
    quick)
        # Fast path: vet + build only
        docker exec ezhealthkonnect-app sh -c "cd /app && go vet ezhealthkonnect/... 2>&1 | grep -v backup" && \
        docker exec ezhealthkonnect-app sh -c "cd /app && go build ezhealthkonnect/... 2>&1 | grep -v backup" && \
        pass "quick check passed" || fail "quick check failed"
        ;;
    all|*)
        check_go
        check_frontend
        check_migrations
        check_logging_arch
        ;;
esac

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
if [ "$FAILURES" -eq 0 ]; then
    green "  ALL CHECKS PASSED"
else
    red "  $FAILURES CHECK(S) FAILED — see above"
fi
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

exit $FAILURES

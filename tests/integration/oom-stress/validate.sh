#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# Validate OOM stress test: wait for a scan job, follow it, report outcome.
#
# Exit codes:
#   0 — scan completed successfully
#   1 — scan was OOM-killed
#   2 — scan failed (non-OOM)
#   3 — timeout / no job appeared
#
# Usage:
#   ./validate.sh                    # wait for next scheduled job
#   ./validate.sh --trigger          # delete old jobs, wait for fresh scheduled run

set -euo pipefail

NAMESPACE="${NAMESPACE:-mondoo-operator}"
CRONJOB="mondoo-container-scan-oom-stress-test"
LABEL="app=mondoo-container-scan,mondoo_cr=oom-stress-test"
TIMEOUT="${OOM_STRESS_TIMEOUT:-900}"
POLL=5

log()  { echo "[$(date '+%H:%M:%S')] $*"; }
warn() { echo "[$(date '+%H:%M:%S')] WARNING: $*"; }
fail() { echo "[$(date '+%H:%M:%S')] FAIL: $*"; }
pass() { echo "[$(date '+%H:%M:%S')] PASS: $*"; }

list_pods() {
  kubectl get pods -n "$NAMESPACE" -l "$LABEL" \
    -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null || true
}

# --- pre-flight ---

if ! kubectl get cronjob "$CRONJOB" -n "$NAMESPACE" &>/dev/null; then
  fail "CronJob $CRONJOB not found in $NAMESPACE. Run terraform apply first."
  exit 3
fi

if [[ "${1:-}" == "--trigger" ]]; then
  log "Deleting old scan jobs and waiting for pods to drain..."
  kubectl delete jobs -n "$NAMESPACE" -l "mondoo_cr=oom-stress-test" --ignore-not-found 2>/dev/null
  # wait for pods from deleted jobs to actually disappear
  for i in $(seq 1 30); do
    remaining=$(list_pods | grep -c . || true)
    if (( remaining == 0 )); then break; fi
    sleep 1
  done
fi

# snapshot any pods that still exist — we'll ignore these
EXISTING_PODS=$(list_pods)

log "Waiting for a new scan pod (timeout ${TIMEOUT}s)..."
schedule=$(kubectl get cronjob "$CRONJOB" -n "$NAMESPACE" -o jsonpath='{.spec.schedule}' 2>/dev/null || true)
[[ -n "$schedule" ]] && log "CronJob schedule: $schedule"

START=$(date +%s)
POD=""

while true; do
  elapsed=$(( $(date +%s) - START ))
  if (( elapsed > TIMEOUT )); then
    fail "No new scan pod appeared within ${TIMEOUT}s"
    exit 3
  fi

  while IFS= read -r p_name; do
    [[ -z "$p_name" ]] && continue
    if echo "$EXISTING_PODS" | grep -qxF "$p_name" 2>/dev/null; then
      continue
    fi
    POD="$p_name"
    break
  done < <(list_pods)

  if [[ -n "$POD" ]]; then
    phase=$(kubectl get pod "$POD" -n "$NAMESPACE" -o jsonpath='{.status.phase}' 2>/dev/null || true)
    log "Found pod $POD ($phase)"
    break
  fi

  printf "\r  waiting for CronJob to fire... (%ds)" "$elapsed"
  sleep "$POLL"
done

echo ""

# --- follow the pod ---

log "Following pod $POD..."
prev_uploaded=0

while true; do
  elapsed=$(( $(date +%s) - START ))
  if (( elapsed > TIMEOUT )); then
    fail "Pod still running after ${TIMEOUT}s"
    exit 3
  fi

  phase=$(kubectl get pod "$POD" -n "$NAMESPACE" -o jsonpath='{.status.phase}' 2>/dev/null || echo "Unknown")

  reason=$(kubectl get pod "$POD" -n "$NAMESPACE" \
    -o jsonpath='{.status.containerStatuses[0].state.terminated.reason}' 2>/dev/null || true)
  exit_code=$(kubectl get pod "$POD" -n "$NAMESPACE" \
    -o jsonpath='{.status.containerStatuses[0].state.terminated.exitCode}' 2>/dev/null || true)

  uploaded=$(kubectl logs "$POD" -n "$NAMESPACE" 2>/dev/null \
    | grep -c 'successfully uploaded' || true)
  uploaded=$(( ${uploaded:-0} + 0 ))

  if (( uploaded > prev_uploaded )); then
    log "  scanned $uploaded image(s)..."
    prev_uploaded=$uploaded
  fi

  # --- terminal states ---

  if [[ "$reason" == "OOMKilled" || "$exit_code" == "137" ]]; then
    echo ""
    echo "════════════════════════════════════════"
    fail "Scanner was OOM-killed after $uploaded image(s) (${elapsed}s)"
    echo "  pod:    $POD"
    echo "  reason: ${reason:-exit 137}"
    echo "  limit:  $(kubectl get pod "$POD" -n "$NAMESPACE" \
      -o jsonpath='{.spec.containers[0].resources.limits.memory}' 2>/dev/null || echo '?')"
    echo "════════════════════════════════════════"
    echo ""
    log "Last 10 log lines:"
    kubectl logs "$POD" -n "$NAMESPACE" --tail=10 2>/dev/null || true
    exit 1
  fi

  if [[ "$phase" == "Succeeded" ]]; then
    echo ""
    echo "════════════════════════════════════════"
    pass "Scan completed successfully — $uploaded image(s) in ${elapsed}s"
    echo "════════════════════════════════════════"
    exit 0
  fi

  if [[ "$phase" == "Failed" ]]; then
    # cnspec exits 1 for both policy violations AND fatal errors —
    # check logs to distinguish
    has_ftl=$(kubectl logs "$POD" -n "$NAMESPACE" 2>/dev/null \
      | grep -c 'FTL\|could not find an asset' || true)
    has_ftl=$(( ${has_ftl:-0} + 0 ))
    if [[ "$exit_code" == "1" ]] && (( has_ftl == 0 )) && (( uploaded > 0 )); then
      echo ""
      echo "════════════════════════════════════════"
      pass "Scan completed with findings — $uploaded image(s) in ${elapsed}s"
      echo "════════════════════════════════════════"
      exit 0
    fi
    echo ""
    echo "════════════════════════════════════════"
    warn "Scan failed (not OOM) after $uploaded image(s) (${elapsed}s)"
    echo "  pod:    $POD"
    echo "  reason: ${reason:-unknown}"
    echo "  exit:   ${exit_code:-?}"
    echo "════════════════════════════════════════"
    echo ""
    log "Last 15 log lines:"
    kubectl logs "$POD" -n "$NAMESPACE" --tail=15 2>/dev/null || true
    exit 2
  fi

  sleep "$POLL"
done

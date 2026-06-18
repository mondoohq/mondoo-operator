#!/usr/bin/env bash
# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

# OOM Stress Test Runner
#
# Sets up everything via Terraform, then watches for the scan pod to OOM.
#
# Prerequisites:
#   - Cluster running with mondoo-operator installed
#   - terraform, kubectl available
#   - MONDOO_API_TOKEN or ~/.config/mondoo/mondoo.yml configured
#
# Usage:
#   cd tests/integration/oom-stress
#   cp terraform/terraform.example.tfvars terraform/terraform.tfvars
#   # edit terraform.tfvars with your mondoo_org_id
#   ./run.sh [apply|watch|destroy|status]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TF_DIR="${SCRIPT_DIR}/terraform"

TIMEOUT_SECONDS="${OOM_STRESS_TIMEOUT:-1500}"  # 25 minutes
POLL_INTERVAL=5

log() { echo "[$(date '+%H:%M:%S')] $*"; }

cmd_apply() {
  log "Running terraform apply..."
  cd "$TF_DIR"
  terraform init -upgrade
  terraform apply -auto-approve

  OPERATOR_NS=$(terraform output -raw operator_namespace 2>/dev/null || echo "mondoo-operator")
  MEMORY_LIMIT=$(terraform output -raw scanner_memory_limit)
  IMAGE_COUNT=$(terraform output -raw stress_image_count)

  log ""
  log "Infrastructure ready:"
  log "  Space:        $(terraform output -raw mondoo_space_id)"
  log "  Memory limit: ${MEMORY_LIMIT}"
  log "  Images:       ${IMAGE_COUNT}"
  log ""
  log "Waiting for target pods to be ready..."
  cd "$SCRIPT_DIR"

  TARGET_NS=$(cd "$TF_DIR" && terraform output -raw target_namespace)
  kubectl wait --for=condition=Ready pods \
    -l app.kubernetes.io/part-of=oom-stress-test \
    -n "$TARGET_NS" \
    --timeout=300s 2>/dev/null || log "WARNING: some pods may not be ready yet"

  log "Run './run.sh watch' to monitor the scan, or it will start automatically."
}

cmd_watch() {
  cd "$TF_DIR"
  OPERATOR_NS="mondoo-operator"
  if [ -f terraform.tfstate ]; then
    OPERATOR_NS=$(terraform output -raw operator_namespace 2>/dev/null || echo "mondoo-operator")
  fi

  SCAN_LABEL="app=mondoo-container-scan,mondoo_cr=oom-stress-test"
  START_TIME=$(date +%s)

  log "Watching for scan pod (label: ${SCAN_LABEL})..."
  log "Timeout: ${TIMEOUT_SECONDS}s"
  log ""

  while true; do
    ELAPSED=$(( $(date +%s) - START_TIME ))
    if [ "$ELAPSED" -gt "$TIMEOUT_SECONDS" ]; then
      log "TIMEOUT: No scan pod terminated within ${TIMEOUT_SECONDS}s"
      cmd_status
      exit 1
    fi

    POD_COUNT=$(kubectl get pods -n "$OPERATOR_NS" -l "$SCAN_LABEL" --no-headers 2>/dev/null | wc -l | tr -d ' ')

    if [ "$POD_COUNT" -eq 0 ]; then
      CJ_EXISTS=$(kubectl get cronjob -n "$OPERATOR_NS" -l "mondoo_cr=oom-stress-test" --no-headers 2>/dev/null | wc -l | tr -d ' ')
      if [ "$CJ_EXISTS" -eq 0 ]; then
        log "  No CronJob yet... (${ELAPSED}s) — check operator logs if this persists"
      else
        log "  CronJob exists, waiting for Job to spawn... (${ELAPSED}s)"
      fi
      sleep "$POLL_INTERVAL"
      continue
    fi

    while IFS= read -r POD_NAME; do
      [ -z "$POD_NAME" ] && continue
      POD_PHASE=$(kubectl get pod -n "$OPERATOR_NS" "$POD_NAME" -o jsonpath='{.status.phase}' 2>/dev/null || echo "Unknown")

      TERM_REASON=$(kubectl get pod -n "$OPERATOR_NS" "$POD_NAME" \
        -o jsonpath='{.status.containerStatuses[0].state.terminated.reason}' 2>/dev/null || true)
      EXIT_CODE=$(kubectl get pod -n "$OPERATOR_NS" "$POD_NAME" \
        -o jsonpath='{.status.containerStatuses[0].state.terminated.exitCode}' 2>/dev/null || true)
      LAST_REASON=$(kubectl get pod -n "$OPERATOR_NS" "$POD_NAME" \
        -o jsonpath='{.status.containerStatuses[0].lastState.terminated.reason}' 2>/dev/null || true)

      OOM_KILLED="false"
      if [ "$TERM_REASON" = "OOMKilled" ] || [ "$LAST_REASON" = "OOMKilled" ] || [ "$EXIT_CODE" = "137" ]; then
        OOM_KILLED="true"
      fi

      if [ "$OOM_KILLED" = "true" ]; then
        COMPLETED=$(kubectl logs -n "$OPERATOR_NS" "$POD_NAME" --tail=10000 2>/dev/null | grep -c "successfully uploaded" || echo 0)
        log ""
        log "=== OOM DETECTED ==="
        log "Pod:      $POD_NAME"
        log "Phase:    $POD_PHASE"
        log "Elapsed:  ${ELAPSED}s"
        log "Images:   $COMPLETED completed before OOM"
        log ""
        log "PASSED: scanner OOM-killed as expected."
        exit 0
      fi

      if [ "$POD_PHASE" = "Succeeded" ]; then
        log ""
        log "=== SCAN COMPLETED WITHOUT OOM ==="
        log "Pod $POD_NAME completed successfully."
        log "Lower scanner_memory_limit or add more images."
        log ""
        log "FAILED: no OOM observed."
        exit 1
      fi

      if [ "$POD_PHASE" = "Failed" ] && [ "$OOM_KILLED" = "false" ]; then
        log ""
        log "=== SCAN FAILED (not OOM) ==="
        kubectl describe pod -n "$OPERATOR_NS" "$POD_NAME" 2>/dev/null | tail -15 || true
        log ""
        log "INCONCLUSIVE: pod failed for non-OOM reason."
        exit 2
      fi
    done < <(kubectl get pods -n "$OPERATOR_NS" -l "$SCAN_LABEL" --no-headers -o custom-columns='NAME:.metadata.name' 2>/dev/null)

    log "  Scan running... (${ELAPSED}s, phase: ${POD_PHASE:-unknown})"
    sleep "$POLL_INTERVAL"
  done
}

cmd_status() {
  cd "$TF_DIR"
  OPERATOR_NS="mondoo-operator"

  log "=== Cluster State ==="
  echo ""
  echo "--- CronJobs ---"
  kubectl get cronjobs -n "$OPERATOR_NS" -l "mondoo_cr=oom-stress-test" 2>/dev/null || echo "(none)"
  echo ""
  echo "--- Scan Pods ---"
  kubectl get pods -n "$OPERATOR_NS" -l "app=mondoo-container-scan,mondoo_cr=oom-stress-test" -o wide 2>/dev/null || echo "(none)"
  echo ""
  echo "--- Target Pods ---"
  TARGET_NS=$(terraform output -raw target_namespace 2>/dev/null || echo "oom-stress-targets")
  kubectl get pods -n "$TARGET_NS" -l "app.kubernetes.io/part-of=oom-stress-test" --no-headers 2>/dev/null | wc -l | xargs -I{} echo "{} target pods"
  echo ""
  echo "--- MondooAuditConfig ---"
  kubectl get mondooauditconfig -n "$OPERATOR_NS" oom-stress-test -o yaml 2>/dev/null | head -30 || echo "(not found)"
  echo ""
  echo "--- Operator Logs (last 10) ---"
  kubectl logs -n "$OPERATOR_NS" deploy/mondoo-operator-controller-manager --tail=10 2>/dev/null || echo "(no operator)"
}

cmd_destroy() {
  log "Destroying terraform resources..."
  cd "$TF_DIR"
  terraform destroy -auto-approve
  log "Done."
}

# --- Main ---

case "${1:-apply-and-watch}" in
  apply)
    cmd_apply
    ;;
  watch)
    cmd_watch
    ;;
  apply-and-watch)
    cmd_apply
    cmd_watch
    ;;
  status)
    cmd_status
    ;;
  destroy)
    cmd_destroy
    ;;
  *)
    echo "Usage: $0 [apply|watch|apply-and-watch|status|destroy]"
    echo ""
    echo "  apply-and-watch  (default) Provision + watch for OOM"
    echo "  apply            Provision infrastructure only"
    echo "  watch            Watch for scan pod OOM (after apply)"
    echo "  status           Show current cluster state"
    echo "  destroy          Tear down all resources"
    exit 1
    ;;
esac

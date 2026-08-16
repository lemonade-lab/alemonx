# ALemonX Agent Release Acceptance Report

> 中文版：[../agent-release-report.md](../agent-release-report.md)。

## Current capabilities

- `TaskService` unifies task creation, start, wait, cancel, and resume entry points.
- `TaskPlan` advances serially through `understand → implement → verify`; a failed verification stays on the current step.
- `GoalRun` supports queued/running/terminal states, restart compensation, and goal-level mutual exclusion.
- Checkpoints, events, snapshots, and reports use local atomic persistence; the legacy chat interface remains compatible.
- The Ops Center supports PM2 error fingerprint deduplication, Incidents, Todos, MaintenanceRuns, project policies, and metrics queries.
- Automatic maintenance uses a project whitelist: `observe` by default, only explicitly authorized projects can enter `auto`; high-risk decisions always go to a human.
- After an automatic task finishes, it enters an observation window; recurring errors stop auto-fix and attempt snapshot rollback, and on failure enter `recovery_required`/Todo.
- The Ops Center provides real-time events, todos, maintenance records, policies, pause collection, and an emergency stop entry.
- `ServerRuntime` provides idempotent Shutdown; on SIGINT/SIGTERM the main process stops background loops, pauses running tasks, and flushes persisted state.
- `OpsPolicy` budget gates cover tokens, PM2 actions, and retries; exceeding the budget turns into a Todo, and repeated failures automatically trip the breaker to `strict`.
- PM2 health signals, process exits, and log events are persisted uniformly; Webhook alerts and Prometheus metric export are supported.

## Ops state machine and canary rules

```text
PM2 logs → fingerprint dedup → Incident triaged → AI decision
  → auto_fix → TaskPlan/Reviewer → observing → resolved
  → failure, recurrence, or high risk → todo / recovery_required
```

- Project policies are stored in `incidents/policy-*.json`; `autoAllowed` is the whitelist gate.
- `OpsMonitor` persists event fingerprints to `seen-events.json`, so a restart does not re-wake the AI for the same event.
- `FailureCircuitBreak` automatically switches to `strict` and revokes the whitelist after reaching its threshold.
- Unfinished MaintenanceRuns are reconciled at startup; missing tasks or snapshot conflicts never replay write operations automatically.
- `GET /api/v1/ops/metrics/prometheus` exports incident, maintenance success, rollback, and MTTR metrics.
- Webhooks are configured with `ALX_OPS_WEBHOOK_URL`; notification failures retry asynchronously and never block agent tasks.

## Enabling online and emergency stop

1. For production, set `ALX_DEPLOYMENT=production` and enable local authentication; ops data defaults to SQLite (when `ALX_OPS_STORAGE` is unset or `sqlite`). Missing configuration only prints a reminder and does not refuse startup (the canary readiness report marks it as not ready).
2. Keep the project in `observe` first and confirm that PM2 logs, event aggregation, and the todo pipeline work.
3. An admin switches a project to `canary` with a reason under Robot Directory → Run → AI Ops. Initially only restart/reload is allowed; code modification is forbidden.
4. After observing stable MTTR, rollback rate, wrong-fix rate, alert delivery rate, and lease anomalies for 24 hours, an admin may confirm again to enable small-scope code fixes; it never upgrades to `auto` automatically.
5. On anomalies use Emergency Stop, or call `POST /api/v1/ops/monitor/emergency-stop`; resume with `POST /api/v1/ops/monitor/resume`.
- SSE supports `Last-Event-ID` resumption; write operations re-enter the `ask` permission confirmation.

## Production auto-maintenance gates

- The new task API defaults to `plan_pending`; the legacy chat interface implicitly approves only for compatibility. Auto-maintenance is approved only when canary/auto, low risk, an unedited plan, and the project whitelist are all satisfied.
- `verificationCommand` is a hard gate for the observation phase: after implementation, the controlled verification command, `agent_verify`, and the Reviewer must all pass; any failure rolls back the snapshot, creates a Todo, and keeps the verification output and report.
- All PM2 write operations (start/stop/restart/reload/delete) pass through fencing leases, emergency stop, and budget checks; read-only status/logs never request a write lease.
- File batch collection and streaming fallback keep separate file/stream cursors. The streaming consumer only activates when batch collection fails; after recovery is detected it switches back automatically to avoid duplicate consumption.
- Alerts are only delivered through a persistent queue; the standalone `AlertDeliveryWorker` holds the delivery lease, reclaims stuck `sending` items after one minute, and retries with exponential backoff. Metrics are kept in 5-minute buckets for 90 days; the JSON API and Prometheus share the same snapshot.

## Entry compatibility matrix

| Entry | Execution path | Compatibility |
| --- | --- | --- |
| `/api/v1/agent/tasks` | TaskService → TaskManager | New tasks need plan approval |
| `/api/v1/agent/chat` | TaskService → Wait | Keeps `{answer, sessionId}` |
| `/api/v1/agent/chat?stream=1` | TaskService → SSE | Keeps the old event format |
| Goal manual run | GoalRun queued → TaskService | Forces ask |
| Goal scheduled run | Scheduler → GoalRun queued → TaskService | Forces ask |

## Acceptance commands

```bash
make test-agent
make test-all
make lint
make build-frontend
git diff --check
```

Test environments can inject temporary directories/listen addresses with `ALX_TEST_CACHE_DIR`, `GOCACHE`, and `TEST_LISTEN_ADDR` to avoid writing to user caches or binding an IPv6 loopback address.

## Known limitations

- Tasks only run while the current ALemonX process is alive; after a restart they pause and require explicit user resume.
- Workspace is the default isolation mode; worktree remains optional.
- Scheduled tasks do not automatically receive high-risk write permissions.
- Real vendors are not part of automated tests; integration tests should use a fake provider.
- PM2 health signals currently come from controlled `jlist/logs` polling, not the PM2 daemon's native event stream.
- The initial Webhook is a generic JSON adapter without vendor-specific on-call escalation policies.
- PM2 batch collection uses real file device/inode/offset via `robot.PM2LogBatchSource` and only starts the streaming fallback when batch reads fail; it still falls back to the PM2 streaming interface when log paths cannot be obtained.
- SQLite v2 persists core searchable fields of events, todos, maintenance, policies, alerts, audit, cursors, budgets, and leases as columns while keeping payloads for compatibility with older checkpoints/records; deeper nested fields still evolve in payload.
- `AlertDeliveryWorker` is a single in-process consumer; splitting it into a standalone deployment unit for cross-region or independent scaling is future work.
- Metrics aggregate in 5-minute buckets and are retained for 90 days; high-cardinality fingerprints should still be limited by project policy and log throttling.
- Multi-instance leases have transactional race protection and fencing tokens; an old instance cannot keep executing PM2 writes after token changes, lease expiry, or emergency stop. Production still recommends a single-primary canary rollout first.

## Next-stage production reliability increments

- `OpsStore` now exposes its persistence boundary through `OpsRepository`; production can use the pure-Go SQLite adapter based on `modernc.org/sqlite`, development/test can fall back to JSON, and the original backup is kept before migration; it does not depend on a system `sqlite3` command or CGO.
- Monitoring instances use persistent leases to avoid multiple instances consuming logs and triggering auto-fix at the same time.
- Policy supports the `canary` gray mode; dry-run or single human approval can run before auto-maintenance.
- Ops write operations support viewer/operator/approver/admin role headers and write rejections, approvals, rollbacks, policy changes, and emergency stops into `audit.jsonl`.
- Alerts persist to `alert-*.json` and support acknowledgement, silencing, and auditable queries after recovery.
- Maintenance runs save safety, verification, goal-completion, and unrelated-diff scores for later evaluation.
- The SQLite adapter provides schema metadata, indexes, and explicit close; tasks support `Idempotency-Key` to avoid duplicate tasks from request retries; PM2 project/process cursors persist so monitoring can continue from the last collection position.
- `OpsMonitor`, `GoalScheduler`, and auto-write tasks use the unified `LeaseManager`; leases include creation, renewal, and expiry times, and failed renewal stops further execution. Alert retry counts and backoff intervals are configurable per severity policy.
- SQLite core entities are transactionally double-written into explicit business tables and the compatibility records table; project write tasks, `GoalScheduler`, and `OpsMonitor` are all inside the lease boundary.
- This round switched SQLite business entities to the primary read path and added stable reverse-chronological queries for Incident, Todo, Maintenance, Policy, Alert, and LogCursor; the compatibility records table serves only as migration/fallback.
- SQLite leases moved to conditional transactional updates inside the `leases` table, supporting expired takeover, renewal-failure detection, and owner validation; sqlite lease sidecar files are no longer created.
- Auto-maintenance checks queued/running/fixing/verifying/observing records per Incident; repeated analysis never starts a second write task for the same Incident.
- Failed alert deliveries persist `delivery_failed`, retry count, next attempt time, and error reason; `OpsMonitor` polling resumes records that have reached their retry time.
- Both SQLite and JSON provide `AlertQueue`; failed deliveries can be claimed, acknowledged, or requeued; the SQLite queue uses transactional updates in the `alert_deliveries` table.
- Leases now expose `GetLease`/`ListLeases` and fencing tokens; project write tasks validate token changes during renewal and pause proactively.
- A new `LogBatch`/`LogBatchSource` compatibility extension lets log sources with file metadata provide real path, device, inode, and offset; the legacy PM2 streaming interface remains the fallback.
- `robot.PM2LogBatchSource` exposes error log paths from PM2 jlist and handles rotation, truncation, inode/device, and offset; `OpsMonitor` prefers batch reads and falls back to the old streaming source.
- Automatic restart/reload goes through `GuardedPM2Executor`, which validates the project fencing token, emergency-stop state, and PM2 budget.
- New JSON/SQLite `MetricsRepository` and the `ops_metrics` table accumulate event metrics atomically and share a Snapshot between JSON and Prometheus.
- A new `/api/v1/ops/metrics/query` query entry exists; `GuardedPM2Executor` uniformly protects restart/reload and writes audit records and PM2 failure results.

Ops data defaults to SQLite (`ops.db`); set `ALX_OPS_SQLITE_PATH` to choose the database file. Setting `ALX_OPS_STORAGE=json` (or `file`) forces JSON file storage. At startup a pure-Go migrator imports existing JSON records into SQLite (originals are backed up to `incidents/backups/`); if migration or opening fails, it falls back to JSON mode and does not start auto-write tasks.

Canary readiness report: admins query `GET /api/v1/ops/canary-readiness?root=<robot-dir>`; the page lives under Robot Directory → Run → AI Ops. The report requires production auth, SQLite, whitelist, fenced PM2 permissions, verification contract, alert worker/receiver, and no triggered emergency stop to be ready; it never enables canary automatically.

When enabling production, first use `observe`, then select a single-project `canary`, and only switch to `auto` after confirming alert delivery, lease takeover, dry-run, and human takeover flows.

## Rollback notes

- Task-level snapshots keep pre-change data and validate the current file hash before rollback.
- Checkpoints, events, and reports are not deleted by rollback, preserving audit and re-recovery ability.
- Worktree merges require user confirmation; conflicts refuse silent overwrite.

## Acceptance results for this round

- `make lint`: passed; Go vet and frontend ESLint have no errors/warnings.
- `make test-all`: uses temporary `ALX_TEST_CACHE_DIR`, `GOCACHE`, and `GOTMPDIR`; release machines must reserve at least 1 GiB of test temp space, and `make test-space` fails explicitly when space is insufficient.
- `make build-frontend`: passed.
- `git diff --check`: passed.
- New failure fixtures cover budget exhaustion, persisted deduplication, Webhook failure isolation, and idempotent Runtime Shutdown.
- Full-repo tests, `go test -race ./internal/agent ./internal/web`, and frontend lint/build all pass.

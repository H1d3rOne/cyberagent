# CyberAgent v0.1.2 Release Candidate Diff Inventory

Date: 2026-06-09
Scope: P0.5-N228 `release_candidate_diff_archive_and_tag_readiness_audit`

## Outcome

The current release-candidate workspace is inventoried but **not tag-ready**.
Deprecated runtime API compatibility remains unchanged:

- keep `AttachLLMTurnRunner` default-attached
- keep `AttachTurnExecutor` default-attached
- do not introduce `CYBERAGENT_RUNTIME_COMPAT`
- do not introduce `CYBERAGENT_DEPRECATED_RUNTIME_API`
- keep `CYBERAGENT_RUNTIME` as the whole-engine rollback switch only

## Snapshot commands

```bash
git status --short
git diff --stat
```

## Snapshot summary

The snapshot taken for this archive before creating this document contained:

- 48 tracked modified paths
- 32 untracked paths
- 80 total `git status --short` entries
- tracked diff stat: 48 files changed, 11521 insertions(+), 1413 deletions(-)

This document itself is part of the N228 archive artifact and should be included in the review/stage/commit set before any release tag.

## Gate decision

The release candidate should not advance to deprecated runtime API opt-in flag or default attachment reduction until all gates below are true:

- [ ] intended tracked and untracked files are reviewed against this inventory
- [ ] generated/derived artifacts are separated from source changes where needed
- [ ] new compatibility audit files are intentionally staged, including `internal/interfaces/daemon/agent_runtime_compatibility.go` and `internal/interfaces/daemon/agent_runtime_compatibility_test.go`
- [ ] full release regression is rerun after the final intended diff is staged
- [ ] `git status --short` contains only intentional release-candidate paths before tagging
- [ ] deprecated runtime API defaults are deliberately changed only in a later gate with explicit release/API documentation

## Tracked modified paths

```text
 M docs/CyberAgent-Release-Checklist.md
 M docs/CyberAgent-Release-Plan.md
 M docs/CyberAgent-TODO.md
 M docs/api/v0.1.2-api-contract.md
 M docs/release-notes/v0.1.2.md
 M internal/agent/runtime/agent_loop_action_batch.go
 M internal/agent/runtime/agent_loop_action_execution_payload.go
 M internal/agent/runtime/agent_loop_action_execution_payload_test.go
 M internal/agent/runtime/agent_loop_action_gate_result.go
 M internal/agent/runtime/agent_loop_action_gate_result_test.go
 M internal/agent/runtime/agent_loop_runner_payload.go
 M internal/agent/runtime/agent_loop_runner_payload_test.go
 M internal/agent/runtime/agent_loop_session_mutation_metadata.go
 M internal/agent/runtime/agent_loop_session_mutation_metadata_test.go
 M internal/agent/runtime/agent_loop_tool_service_binding.go
 M internal/agent/runtime/agent_loop_tool_service_binding_test.go
 M internal/agent/runtime/agent_loop_turn_execution_metadata.go
 M internal/agent/runtime/agent_loop_turn_execution_metadata_test.go
 M internal/agent/runtime/llm_turn_runner.go
 M internal/agent/runtime/runtime.go
 M internal/agent/runtime/turn_executor.go
 M internal/interfaces/daemon/agent_action_gate_result_test.go
 M internal/interfaces/daemon/agent_common.go
 M internal/interfaces/daemon/agent_helpers_test.go
 M internal/interfaces/daemon/agent_llm_stage_runner.go
 M internal/interfaces/daemon/agent_llm_stage_runner_test.go
 M internal/interfaces/daemon/agent_llm_turn_runner.go
 M internal/interfaces/daemon/agent_loop_runner.go
 M internal/interfaces/daemon/agent_loop_runner_adapter_test.go
 M internal/interfaces/daemon/agent_planner_loop.go
 M internal/interfaces/daemon/agent_prompt_dispatch.go
 M internal/interfaces/daemon/agent_registry_dispatch.go
 M internal/interfaces/daemon/agent_registry_dispatch_test.go
 M internal/interfaces/daemon/agent_registry_handlers.go
 M internal/interfaces/daemon/agent_run_handlers.go
 M internal/interfaces/daemon/agent_runtime.go
 M internal/interfaces/daemon/agent_schedule_web.go
 M internal/interfaces/daemon/agent_send_message_lifecycle_test.go
 M internal/interfaces/daemon/agent_send_message_response_merge_test.go
 M internal/interfaces/daemon/agent_session_flow.go
 M internal/interfaces/daemon/agent_session_helpers.go
 M internal/interfaces/daemon/agent_session_mutation_result_adapter.go
 M internal/interfaces/daemon/agent_session_mutator.go
 M internal/interfaces/daemon/agent_session_mutator_test.go
 M internal/interfaces/daemon/agent_tool_service_binding.go
 M internal/interfaces/daemon/agent_tool_service_binding_test.go
 M internal/interfaces/daemon/agent_turn_executor.go
 M internal/interfaces/daemon/agent_turn_executor_test.go
```

## Untracked paths

```text
?? internal/agent/runtime/agent_loop_action_executor.go
?? internal/agent/runtime/agent_loop_action_executor_test.go
?? internal/agent/runtime/agent_loop_action_services.go
?? internal/agent/runtime/agent_loop_action_services_test.go
?? internal/agent/runtime/agent_loop_planner_action_execution_result.go
?? internal/agent/runtime/agent_loop_planner_action_execution_result_test.go
?? internal/agent/runtime/agent_loop_replan_cycle_append.go
?? internal/agent/runtime/agent_loop_replan_cycle_append_test.go
?? internal/agent/runtime/agent_loop_turn_execution_request.go
?? internal/agent/runtime/agent_loop_turn_execution_request_test.go
?? internal/interfaces/daemon/agent_action_executor.go
?? internal/interfaces/daemon/agent_daemon_status_action_service.go
?? internal/interfaces/daemon/agent_daemon_status_action_service_test.go
?? internal/interfaces/daemon/agent_loop_runner_test.go
?? internal/interfaces/daemon/agent_network_request_action_service.go
?? internal/interfaces/daemon/agent_network_request_action_service_test.go
?? internal/interfaces/daemon/agent_port_scanner_action_service.go
?? internal/interfaces/daemon/agent_port_scanner_action_service_test.go
?? internal/interfaces/daemon/agent_replan_action_continuation_test.go
?? internal/interfaces/daemon/agent_run_context_approval_action_service.go
?? internal/interfaces/daemon/agent_run_context_approval_action_service_test.go
?? internal/interfaces/daemon/agent_runtime_compatibility.go
?? internal/interfaces/daemon/agent_runtime_compatibility_test.go
?? internal/interfaces/daemon/agent_schedule_action_service.go
?? internal/interfaces/daemon/agent_schedule_action_service_test.go
?? internal/interfaces/daemon/agent_tool_service_binding_compat_test.go
?? internal/interfaces/daemon/agent_web_scan_action_service.go
?? internal/interfaces/daemon/agent_web_scan_action_service_test.go
?? internal/interfaces/daemon/agent_web_search_action_service.go
?? internal/interfaces/daemon/agent_web_search_action_service_test.go
?? internal/interfaces/daemon/agent_workspace_action_service.go
?? internal/interfaces/daemon/agent_workspace_action_service_test.go
```

## Tracked diff stat

```text
 docs/CyberAgent-Release-Checklist.md               |   34 +
 docs/CyberAgent-Release-Plan.md                    |   21 +
 docs/CyberAgent-TODO.md                            | 1525 +++++++++++-
 docs/api/v0.1.2-api-contract.md                    |   23 +
 docs/release-notes/v0.1.2.md                       |    8 +
 internal/agent/runtime/agent_loop_action_batch.go  |   35 +
 .../runtime/agent_loop_action_execution_payload.go |   11 +
 .../agent_loop_action_execution_payload_test.go    |   47 +
 .../agent/runtime/agent_loop_action_gate_result.go |   86 +
 .../runtime/agent_loop_action_gate_result_test.go  |  253 ++
 .../agent/runtime/agent_loop_runner_payload.go     |   11 +
 .../runtime/agent_loop_runner_payload_test.go      |    8 +
 .../agent_loop_session_mutation_metadata.go        |  261 +++
 .../agent_loop_session_mutation_metadata_test.go   |  276 +++
 .../runtime/agent_loop_tool_service_binding.go     |   21 +-
 .../agent_loop_tool_service_binding_test.go        |   21 +
 .../runtime/agent_loop_turn_execution_metadata.go  |   27 +
 .../agent_loop_turn_execution_metadata_test.go     |   88 +
 internal/agent/runtime/llm_turn_runner.go          |   14 +
 internal/agent/runtime/runtime.go                  |   21 +-
 internal/agent/runtime/turn_executor.go            |   21 +
 .../daemon/agent_action_gate_result_test.go        |    6 +-
 internal/interfaces/daemon/agent_common.go         |  155 ++
 internal/interfaces/daemon/agent_helpers_test.go   |  162 +-
 .../interfaces/daemon/agent_llm_stage_runner.go    |  250 +-
 .../daemon/agent_llm_stage_runner_test.go          |  264 +++
 .../interfaces/daemon/agent_llm_turn_runner.go     |   23 +-
 internal/interfaces/daemon/agent_loop_runner.go    |  361 ++-
 .../daemon/agent_loop_runner_adapter_test.go       |  141 +-
 internal/interfaces/daemon/agent_planner_loop.go   |  194 +-
 .../interfaces/daemon/agent_prompt_dispatch.go     |  122 +-
 .../interfaces/daemon/agent_registry_dispatch.go   |  170 +-
 .../daemon/agent_registry_dispatch_test.go         |  853 ++++++-
 .../interfaces/daemon/agent_registry_handlers.go   |  398 ++--
 internal/interfaces/daemon/agent_run_handlers.go   |  143 +-
 internal/interfaces/daemon/agent_runtime.go        |    1 +
 internal/interfaces/daemon/agent_schedule_web.go   |   36 +-
 .../daemon/agent_send_message_lifecycle_test.go    |   27 +-
 .../agent_send_message_response_merge_test.go      |   44 +-
 internal/interfaces/daemon/agent_session_flow.go   |   68 +-
 .../interfaces/daemon/agent_session_helpers.go     |   16 +-
 .../agent_session_mutation_result_adapter.go       |  120 +-
 .../interfaces/daemon/agent_session_mutator.go     | 1007 ++++----
 .../daemon/agent_session_mutator_test.go           |  660 +++++-
 .../daemon/agent_tool_service_binding.go           |  335 ++-
 .../daemon/agent_tool_service_binding_test.go      |  177 +-
 internal/interfaces/daemon/agent_turn_executor.go  | 1959 +++++++++++++++-
 .../interfaces/daemon/agent_turn_executor_test.go  | 2430 ++++++++++++++++++--
 48 files changed, 11521 insertions(+), 1413 deletions(-)
```

## Next boundary

Next boundary: `release_candidate_stage_commit_and_full_regression_gate`

The next step should turn this inventory into a final stage/commit/tag-readiness gate. Until that gate passes, deprecated runtime API flag/reduction remains deferred and compatibility defaults stay preserved.

# Run references and facts

Canonical definitions are in `run-context.schema.json`; AG-UI aliases and event dispatch are in `../fixtures/agui.schema.json`. These optional extensions preserve old clients. Generic AG-UI `context`, `tools`, `state` and unknown forwarded properties remain ignored and never become backend instructions or tool authority.

## References sent with a turn

The first-party composer sends `forwardedProps.ipfs.context` as an array of `ContextReference`. Each entry has an opaque stable `id`, claimed display `provenance` (`page` or `user`), and at least one of `route`, `cid`, or `path`. A path also requires `path_kind`: `ipfs` for logical `/ipfs/<CID>` or `/ipns/<name>` content paths, or `mfs` for the connected node's logical MFS. Even `mfs:/etc/shadow` means a node MFS reference; it never names a file on the server's host.

Validate before either built-in or UAR receives the turn:

- At most 16 entries, unique IDs, and at most 16,384 bytes in the raw UTF-8 JSON array value, including JSON escapes and whitespace. The client also bounds its serialized array before sending. Empty or absent means no references.
- Apply all structural bounds in the schema, including the existing common CID and MfsPath contracts. A MFS reference cannot smuggle a /ipfs/, /ipns/ or /ipld/ content namespace through a mutable-file label. Parse CID strings and IPFS/IPNS path roots with existing syntax parsers without resolving names, fetching content or opening a repository.
- Routes and paths are decoded logical strings, start with a single slash, and contain no controls, backslashes, query/fragment, dot or dot-dot segment, scheme, credentials, or residual percent escape. The page adapter decodes its route once; invalid encoding and any remaining percent escape fail closed. Reject repeated separators and encoded or double-encoded traversal/separators. Do not reinterpret a URL or local-file URI as a reference. If both a CID and an /ipfs/ path root are present, they must identify the same CID.
- Unknown reference fields, invalid branch types, duplicate IDs and overflow yield HTTP 422 with the existing `invalid_argument` ToolError envelope, no stream and no backend call. Messages identify the invalid branch/index or limit without echoing reference contents or credentials. There is no silent truncation or fallback through generic `context`.

The scheme/credential restriction concerns URI and authority representations, such as `file:///etc/passwd`, `https://user:password@host/path` or `//user@host/path`. A logical MFS filename may contain `:` or `@` under the existing MFS contract; these characters alone are not credentials or a scheme. Never reinterpret a decoded logical path as a URL or host filesystem path. Content namespace roots still use the existing IPFS/IPNS syntax validators.

The 422 response uses `Content-Type: application/json` and the existing ToolError body, for example `{"error":{"code":"invalid_argument","message":"forwardedProps.ipfs.context: at most 16 references are allowed.","retryable":false}}` (`examples/context-invalid.json`). The UI keeps the draft/references available for correction and displays the payload error; it does not report a model run, stream failure or successful send. Index/type/syntax messages use the same shape without echoing content. Existing authentication, origin, request-size and thread checks still apply.

References must be visible and removable before send. Removed page references stay absent across every subsequent send until the route changes and visibly introduces a new reference. Removing a reference does not retract text already accepted into the in-memory conversation. A continuation uses its retained accepted turn; a resume with a new nonempty reference array is rejected with 422. An omitted or empty resume array does not erase the retained turn. The inspector shows `context_snapshot` for the inspected run, not the current browser route.

References are untrusted data. The built-in path frames their validated serialized representation as untrusted content using the existing delimiter escaping in `agent_frame.go`; closing-delimiter text cannot escape that frame. The UAR path adds the same bounded framed reference data to the accepted user turn, without promoting it to system instructions. It leaves generic context/tools/state empty and retains server-owned UAR grants. Client properties cannot replace those grants. Actual content is accessed only by existing authorized tools with existing confirmation and egress checks. Apply existing applicable destination/egress policy before forwarding references to either configured backend; the reference never supplies acknowledgment or overrides policy. No implicit file read, CID fetch, name resolution, upload, pin, or content announcement occurs.

## Independent forwarded branches

`IpfsForwardedProps` describes the typed authoring shape, not an instruction to validate the entire namespace atomically. Decode known branches independently. Invalid `context` or `surface` is the 422 payload error above. Missing, malformed, oversized or incompatible `a2ui` disables A2UI alone, preserving a valid reference turn and ordinary tool/text results; see `ui-negotiation.md`. Invalid context still rejects a request with a valid A2UI offer. Unknown generic properties are not forwarded.

Optional `surface: {kind: "chat" | "inbox", instance: <opaque>}` is display attribution only. It cannot select a principal, resolver mechanism, destination, grant or identity binding. Observation and page provenance claims supply no approval authority.

## Facts from the accepted run

Emit standard AG-UI `CUSTOM` named `ipfs.run.facts` with `RunFactsEvent` as its value. Emit at most once for an accepted run: after `RUN_STARTED`, after the immediately following `a2ui.catalog` if negotiated, and before tool/text events. Include `thread_id` and `run_id`; consumers bind both to the current run and ignore a stale or mismatched event. This extension is independent of A2UI. No unknown extra fields are added to the standard `RUN_STARTED` object, where SDK schema parsing may strip them. Existing start-only clients and fixtures remain valid.

Every fact is optional, with a corresponding source in `provenance` if supplied:

| Fact | Authoritative source |
| --- | --- |
| `offered_tools` | Names actually supplied by `Agent.offer(eff)` from the effective policy and retained run snapshot; source `effective_policy_offer`. Never `Server.ToolNames()`. |
| `catalog_size` | Actual loaded Kubo contract size, source `loaded_contract`, scope `kubo_contract`. It is a catalog denominator, not the number enabled by the principal. |
| `system_prompt_digest` | `sha256:` plus the SHA-256 of exact UTF-8 `run.snap.systemPrompt`, including its safety prefix and retained continuation snapshot; source `effective_run_snapshot`. Never rebuild using newer settings or use a key fingerprint. |
| `backend` | Actual selected snapshot, source `effective_run_snapshot`; include model, placement and sanitized host[:port] only when known. Never disclose endpoint credentials, path/query or arbitrary config. |
| `context_snapshot` | Validated references accepted for this turn, source `accepted_turn`, including explicit empty when known none. |
| `memory` | Established current conversation behavior from the snapshot, or a validated backend report. Missing is not an assertion of None. |
| `egress` | Actual effective acknowledged host labels from the snapshot. This projection grants no network permission. |

UAR uses the same optional shape. Its actual model tool set, catalog denominator, final system prompt and memory behavior require `validated_backend_report`; tool facts use `tool_scope: "backend_report"`. Kubo's registry, prompt template or local conversation cannot stand in for them. If no trustworthy report exists, omit those fields. Backend selection and accepted references can still be reported from local facts. No raw UAR CUSTOM/facts passthrough: validate and construct the projection at the trusted adapter. Do not invent prompt token allocations, budget percentages, model capacity, durable memory or local/remote placement. Missing facts render “Not reported by this server”; positively reported empty tool lists or `memory.scope: "none"` are distinct.

Observation-only native-wait resumes may omit facts instead of reconstructing a nonexistent model invocation. New continuation invocations use the retained prompt/backend/context snapshot and actual effective tool offer. Omit the facts event when there are no reportable facts. It is descriptive and never an operation receipt; execution success still requires the D contract's real evidence.

I/1.4 defines optional server-assigned `activity_id` on this event value, on `Interrupt.metadata["ipfs-mcp"]` and on DecisionObservation. A new accepted user message gets a new activity ID; authorized resumed runs retain it. See `../decisions/README.md` for exact correlation and privacy semantics. With `agent.run_facts=false`, this CUSTOM event remains suppressed but interrupt metadata can still associate a decision with its accepted activity. This does not insert custom RUN_STARTED fields or make an activity ID a credential.

## Inspector and configuration

The inspector is read-only and current run only. Overview, Context, Prompt plan and Events remain present when facts are missing. Project actual events to `InspectorEvent` without raw system prompt, credentials, message bodies or sensitive arguments. Keep at most 1,000 entries and 524,288 serialized UTF-8 bytes, dropping oldest entries as needed and showing truncation. Virtualize after 200 rows; reset on inspected run switch. Missing terminal evidence becomes unknown, never Done; approval is not execution success.

`agent.context_references` defaults true; false rejects a nonempty reference branch with 422 before either backend, while ordinary messages work. `agent.run_facts` defaults true; false emits no facts projection/event while ordinary runs work. These do not weaken write confirmation, egress, native prompt or identity checks. D/4.2 owns generated config documentation/default examples and disabled-state integration wiring.

C/1.2 and C/1.3 implement both backend paths. C/2.1 owns generated UI definitions and consumers: resolve these external refs and preserve conditional provenance constraints; do not silently drop unsupported conditions. Author fixtures and source tests now; the phase's final boundary runs them.

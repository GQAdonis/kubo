# Operator decisions — agentic UI implementation

Date: 2026-10-20. Decision owner: the operator in this Codex chat.

These are explicit operator answers, not assumptions. Assessment and the `grill-me` interview remain in progress. No implementation dispatch has occurred.

| ID | Decision | Provenance |
|---|---|---|
| UI-01 | Praxis structure and semantic roles with Promethity values and locally bundled fonts. Confirms PX-02. | Q1, accepted recommendation |
| UI-02 | Deliver all eight canonical Praxis changes across Kubo, Web UI and Desktop, with local real-backend verification. Release publishing and deployment are excluded. | Q2, accepted recommendation |
| UI-03 | Eight canonical task lists define feature scope; prototype governs appearance and interaction. Editable agent profiles, trusted-peer settings, persistent 90-day history and six-character pairing require explicit follow-up changes. | Q3, accepted recommendation |
| UI-04 | Migrate everything using React across all three repositories to React 19. Include necessary dependency, mounting and testing changes as a migration foundation. | Direct operator correction: “One change: Migrate EVERYTHING to React 19. continue” |
| UI-04 superseded | Earlier approval to preserve React 16 shell and isolate React 19 was explicitly superseded by UI-04. | Q4 followed by direct correction |
| UI-05 | Web inbox is scoped to the current authenticated identity across its sessions. No new operator-wide cross-identity visibility. | Q5, accepted recommendation |
| UI-06 | Resolved decisions use bounded in-memory history, up to 200 entries or seven days; disclose reset on agent-server restart. Inspector events cover the current run only. Durable history/replay deferred. | Q6, accepted recommendation |
| UI-07 | Add a protected, read-only event subscription on the existing MCP listener for idle-screen updates. Run-bound AG-UI streams are insufficient. | Q7, accepted recommendation |
| UI-08 | Below 540px use Home, Files, Pins, Activity, More bottom navigation, with a More sheet. Amend the older drawer acceptance task. | Q8, accepted recommendation |
| UI-09 | Context comprises visible removable route/CID/IPFS-path references, handled through existing tools and egress rules. No implicit filesystem reads; local-file uploads and multimodal ingestion deferred. | Q9, accepted recommendation |
| UI-10 | Support built-in and UAR backends. Display actual backend-reported facts; unavailable inspector facts say “Not reported.” No invented prompt-budget calculations. | Q10, accepted recommendation |
| UI-11 | Add the eleventh decision presentation “Failed after approval” for a known execution failure. Done requires a successful outcome receipt; uncertainty remains distinct. No implicit claim of zero effects and no automatic write retry. | Q11, accepted recommendation |
| UI-12 | Inbox approves only owned chat interruptions through existing authenticated resolution. Other decisions direct the person to the authorized host or native prompt; observation grants no new approval authority. | Q12, accepted recommendation |
| UI-13 | Desktop distinguishes “Your decisions on this node” from “Paired local app requests,” with separate counts and authorized destinations. Native requests belonging to other identities stay out of the Web UI. | Q13, accepted recommendation |

## Pending interview frontier

- React 19 migration facts are being assessed by the UI engineer; dependent architecture questions wait for that evidence.
- Confirm shared understanding after remaining migration decisions.

## Engineering findings to carry into analysis

- Broker `executed` is an approval-consumption claim before tool invocation, not a success receipt. Preserve exactly-once consumption and add actual outcome tracking.
- Decision list and feed ownership must use identity binding, including OIDC subject isolation, following ADR-0013.
- Context input is currently ignored by the AG-UI decoder. A visible chip alone does not establish model context; add bounded typed ingestion and untrusted framing.
- Offered-tool and prompt facts must come from the effective run snapshot, not a global catalog.
- MCP App messaging is owned by the ext-apps SDK adapter; `host/port.ts` is a TypeScript interface. Verify compatible origin/source/correlation checks before amending transport requirements.
- Resolver mechanism and resolving surface identity are distinct. Specify their relationship and outcome precedence before implementation.

## Interview contract

The invoked `grill-me` delegates to `/Users/gqadonis/.TOOLS/skills/agents/grilling/SKILL.md`: “Do not act on it until the user confirms you have reached a shared understanding.” Continue assessment and analysis preparation; obtain shared-understanding confirmation before implementation.

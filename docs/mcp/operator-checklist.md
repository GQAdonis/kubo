# Operator checklist (2026-10-13)

What is left for phase `ipfs-mcp-server` and the LKE deployment, in the order to do it. Each item says what to do, why it needs you, and what to send back. Everything else is done and pushed.

## A. Hands-on checks (your Mac)

Close or minimize other apps first. Two of these show a Touch ID prompt, and the HG-1 script sends synthetic keys (now only to the authentication UI).

### A1. HG-1: a same-user process cannot pass Touch ID (`native-confirmation-prompt` 1.5)

- **Why you:** the check is that only a real finger on the sensor approves. An automated run cannot touch the sensor, and the lead's unattended run on 2026-10-13 saw no prompt on screen, so it proved nothing (annotated in `mcp/server/confirm/evidence/hg1-Darwin-20261013T074515Z.txt`).
- **Steps:**
  1. Start a throwaway node: `export IPFS_PATH="$(mktemp -d)"; ipfs init --profile test; ipfs config Addresses.API /ip4/127.0.0.1/tcp/5193; ipfs config Addresses.Gateway /ip4/127.0.0.1/tcp/8293; ipfs daemon --offline &`
  2. With `ipfs` from this repo on `PATH`, run `mcp/server/confirm/hg1/attempt.sh`.
  3. When the Touch ID prompt appears, do not touch the sensor or keyboard. The script tries to click and type into it.
  4. The script prints `PASS`, `INCONCLUSIVE` or `FAIL` and writes `mcp/server/confirm/evidence/hg1-Darwin-<time>.txt`. If it reports `INCONCLUSIVE` because no auth window was found, the log names the frontmost process; rerun with `HG1_AUTH_PROCESSES=<that name>`.
  5. The first run may ask you to allow your terminal under Privacy & Security > Accessibility. Allow it: the point is that even a process allowed to click cannot pass Touch ID.
- **Send back:** the evidence file name.

### A2. Paired Desktop with Touch ID (`desktop-agent-chat` 1.6 and 1.8)

- **Why you:** approving in Desktop requires your Touch ID, and the window behaviour (no focus stealing, arming delay) has to be seen on a real screen.
- **Steps:**
  1. Quit any running IPFS Desktop, then open `~/Applications/IPFS Desktop.app` (installed 2026-10-13: Desktop `0ea60ce`, kubo `0f15c2407`, fork Web UI).
  2. Tray menu > Settings > Desktop Confirmations: tick "Confirm agent writes in this app", then "Pair with ipfs mcp...".
  3. In a terminal: `ipfs mcp pair`, and paste the printed key into the pairing window.
  4. Start an MCP client against `ipfs mcp --confirm=native,desktop` and ask for one write (for example "pin bafkqaaa"). In the Desktop window, check that the window did not take focus, and that Approve stays disabled for about 1.5 s after you click into it. Then approve with Touch ID.
  5. Ask for a second write and deny it.
- **Send back:** two screenshots (approve and deny), and `ipfs pin ls` output showing the approved pin present and the denied one absent.

### A3. WebMCP in Codex and Kimi (`webui-webmcp-integration` 1.4)

- **Why you:** the lead can drive these apps, as it did for the probe, but only while you are not using them. The run on 2026-10-13 stopped because Codex was in active use. Either run it yourself or tell the lead the apps are free.
- **Steps (per host):**
  1. Throwaway node: as in A1, but with ports 5194 and 8294 and without `--offline`. Open `http://127.0.0.1:5194/webui/` in the host's built-in browser.
  2. In the Web UI, go to Settings > Agent tools and turn on agent tools, then writes.
  3. Ask the agent: "Use the IPFS tools on this page: show the node info, then pin bafkqaaa." Approve the pin in the page dialog yourself.
  4. (a) Ask the agent to approve its own next `pin_add` by clicking Approve in the page. Record whether it can. The dialog only accepts trusted clicks after an arming delay, but a host that drives the page with real OS input may still get through. That is the known limit recorded in ADR-0002.
  5. (b) Turn agent tools off, then ask for any tool. It must be declined. Record whether the host still listed the tool.
  6. (c) With the DNSLink toggle off, ask it to read `/ipns/docs.ipfs.tech`; this must be declined. Turn the toggle on and ask again; the read must work.
- **Send back:** the chat transcripts (copy or screenshots) for each host.

### A4. Web UI agent chat (`webui-agent-chat` 1.3)

- **Why you:** the chat sidebar has to be opened, used and confirmed by a person in a browser and in Desktop.
- **Steps:**
  1. Throwaway node as in A3. Then run `ipfs mcp --http --agui --api=/ip4/127.0.0.1/tcp/5194 --llm-base-url http://127.0.0.1:11434/v1 --llm-model qwen3:8b --auth-token-env=IPFS_MCP_TOKEN` with `IPFS_MCP_TOKEN` set to a random value.
  2. In the Web UI, go to Settings > Agent chat. Enable it, set the endpoint to `http://127.0.0.1:5050/agui` and the token to the value from step 1.
  3. In chat, ask "What is this node's ID?", then "Pin bafkqaaa", and confirm the pin in the chat.
  4. Repeat in `~/Applications/IPFS Desktop.app`. Its bundled Web UI has the same settings.
- **Send back:** the transcripts, and `ipfs pin ls` output showing the pin.

### A5. MCP App views in a real host (`mcp-app-views` 1.4)

- **Why you:** the views render only inside a host that supports MCP Apps (Claude Desktop or claude.ai). Adding the server to that host changes your app's configuration, so you should do it, or tell the lead to.
- **Steps:** add `ipfs mcp` (stdio, against a throwaway node) to Claude Desktop. Ask for "show my pins" and "show node status", and check that the views render. Then turn the views off with `--no-app-views` and check that the tools still answer in text.
- **Send back:** screenshots of the views, and one text-only answer.

## B. Deployment to `ipfs.prometheusags.ai` (LKE `know-me`)

PR [Prometheus-AGS/know-me-cluster#21](https://github.com/Prometheus-AGS/know-me-cluster/pull/21) is a draft with the tested manifests and step list. Done by the lead: the image built locally (`ghcr.io/gqadonis/kubo:feat-mcp-0f15c24`, linux/amd64, `sha256:5652e7c3…`), the MCP config checked with the fork binary, and a Velero backup of namespace `ipfs` (Velero's schedules do not cover `ipfs`).

- **B1. Push the image.** **Why you:** it publishes to `ghcr.io/gqadonis`. The lead's token can push, but publishing stays your approval. Say "push the image" and the lead pushes it and fills in the digest.
- **B2. Pull access.** **Why you:** `ghcr-pull` does not exist in namespace `ipfs`, and copying it means handling a registry credential the lead does not read. Either create the secret, or make the package `ghcr.io/gqadonis/kubo` public (then delete the `imagePullSecrets` lines).
- **B3. MCP token.** **Why you:** it is a credential that only you should hold. Run `kubectl --context know-me -n ipfs create secret generic ipfs-mcp-token --from-literal=token="$(openssl rand -hex 32)"` and save the value in your password manager.
- **B4. Merge #21.** **Why you:** it replaces the production image, with a few minutes of gateway downtime. After the merge, the lead watches the rollout and verifies it.
- **B5. Basic-auth clients.** A Python client (`python-requests/2.32.5`) uploaded through the public `/api/v0/add` on 2026-10-12. It now gets 401 until it sends the basic-auth credentials. Only you know which tool that is.

## C. Final sign-off (`phase-boundary-review` 1.8)

- **Why you:** it is your approval of the phase, recorded in KBD. It includes committing `AGENTS.md` under the recorded waiver. Tags and publishing remain separate approvals.

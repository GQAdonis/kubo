#!/usr/bin/env python3
"""Drive `codex app-server` over stdio JSON-RPC and summarize what Codex loaded.

Run with an isolated CODEX_HOME. Argument: the repo root that holds .agents/plugins/marketplace.json.
Prints plugin/read, skills/list (plugin skills only) and mcpServerStatus/list as JSON.
"""
import json, subprocess, sys, os, time, select

repo = sys.argv[1]
env = dict(os.environ)
p = subprocess.Popen(["codex", "app-server", "--listen", "stdio://"], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                     stderr=subprocess.PIPE, env=env, text=True, bufsize=1)
nid = 0
def send(method, params=None, notify=False):
    global nid
    msg = {"jsonrpc": "2.0", "method": method}
    if params is not None: msg["params"] = params
    if not notify:
        nid += 1; msg["id"] = nid
    p.stdin.write(json.dumps(msg) + "\n"); p.stdin.flush()
    return nid
def recv_for(rid, timeout=60):
    end = time.time() + timeout
    while time.time() < end:
        r, _, _ = select.select([p.stdout], [], [], 1)
        if not r: continue
        line = p.stdout.readline()
        if not line: return {"error": "eof"}
        try: m = json.loads(line)
        except Exception: continue
        if m.get("id") == rid: return m
    return {"error": "timeout"}
def call(method, params=None, timeout=60):
    rid = send(method, params); return recv_for(rid, timeout)

out = {}
out["initialize"] = call("initialize", {"clientInfo": {"name": "plugin-probe", "version": "0"}})
send("initialized", None, notify=True)
mp = os.path.join(repo, ".agents/plugins/marketplace.json")
r = call("plugin/read", {"pluginName": "ipfs", "marketplacePath": mp})
out["plugin/read"] = r
r = call("skills/list", {"cwds": [repo], "forceReload": True})
out["skills/list"] = r
r = call("mcpServerStatus/list", {"detail": "full"}, timeout=90)
out["mcpServerStatus/list"] = r

sl = out["skills/list"].get("result", {}).get("data", [])
plug = [x["name"] for d in sl for x in d["skills"] if x.get("pluginId")]
summary = {
 "initialize_codexHome": out["initialize"].get("result", {}).get("codexHome"),
 "plugin_read_id": out["plugin/read"]["result"]["plugin"]["summary"]["id"],
 "plugin_read_installed_enabled": [out["plugin/read"]["result"]["plugin"]["summary"]["installed"], out["plugin/read"]["result"]["plugin"]["summary"]["enabled"]],
 "plugin_read_skills": [x["name"] for x in out["plugin/read"]["result"]["plugin"]["skills"]],
 "plugin_read_mcpServers": out["plugin/read"]["result"]["plugin"]["mcpServers"],
 "skills_list_plugin_skills(pluginId set)": plug,
 "mcpServerStatus/list": [ {"name": x["name"], "pluginId": x["pluginId"], "serverInfo": x["serverInfo"], "toolsError": x["toolsError"], "tool_count": len(x["tools"]), "tools": sorted(x["tools"])} for x in out["mcpServerStatus/list"].get("result", {}).get("data", [])],
}
print(json.dumps(summary, indent=1))
p.terminate()

"""Minimal MCP stdio client for the release smoke test.

usage: smoke_client.py <ipfs-binary> <api-multiaddr>
Starts `ipfs mcp --api <addr>`, initializes, lists tools, calls ipfs_node_info.
Exit 0 only if tools/list is non-empty and ipfs_node_info succeeds.
"""
import json
import subprocess
import sys

binary, api = sys.argv[1], sys.argv[2]
proc = subprocess.Popen([binary, "mcp", "--api", api], stdin=subprocess.PIPE,
                        stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
_id = 0


def send(obj):
    proc.stdin.write(json.dumps(obj) + "\n")
    proc.stdin.flush()


def call(method, params):
    global _id
    _id += 1
    send({"jsonrpc": "2.0", "id": _id, "method": method, "params": params})
    while True:
        line = proc.stdout.readline()
        if not line:
            raise SystemExit("server closed stdout; stderr: " + proc.stderr.read())
        msg = json.loads(line)
        if msg.get("id") == _id and ("result" in msg or "error" in msg):
            return msg
        # ignore notifications and server-initiated requests (no elicitation needed here)


try:
    init = call("initialize", {"protocolVersion": "2025-06-18", "capabilities": {},
                               "clientInfo": {"name": "release-smoke", "version": "0"}})
    print("server:", json.dumps(init["result"].get("serverInfo")))
    send({"jsonrpc": "2.0", "method": "notifications/initialized"})
    tools = call("tools/list", {})["result"]["tools"]
    names = sorted(t["name"] for t in tools)
    print("tool count:", len(names))
    print("tools:", ", ".join(names))
    if not names or "ipfs_node_info" not in names:
        raise SystemExit("ipfs_node_info missing from tools/list")
    res = call("tools/call", {"name": "ipfs_node_info", "arguments": {}})
    if "error" in res or res["result"].get("isError"):
        raise SystemExit("ipfs_node_info failed: " + json.dumps(res))
    print("ipfs_node_info result:", json.dumps(res["result"].get("structuredContent")
                                               or res["result"].get("content")))
finally:
    proc.stdin.close()
    try:
        proc.wait(timeout=10)
    except subprocess.TimeoutExpired:
        proc.kill()

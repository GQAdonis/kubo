#!/bin/sh
# tools.sh: initialize an MCP session with the throwaway ui token and print tool names (token never printed).
U=http://127.0.0.1:5151/mcp
TOK=$(cat /tmp/ui.token)
HD="Content-Type: application/json"
AC="Accept: application/json, text/event-stream"
INIT='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}'
wget -S -O /tmp/init.out --header "$HD" --header "$AC" --header "Authorization: Bearer $TOK" --post-data "$INIT" $U 2>/tmp/init.hdr
SID=$(grep -i 'mcp-session-id' /tmp/init.hdr | sed 's/.*: *//' | tr -d '\r\n ')
wget -q -O /dev/null --header "$HD" --header "$AC" --header "Authorization: Bearer $TOK" ${SID:+--header "Mcp-Session-Id: $SID"} --header "Mcp-Protocol-Version: 2025-06-18" --post-data '{"jsonrpc":"2.0","method":"notifications/initialized"}' $U
wget -q -O /tmp/tools.out --header "$HD" --header "$AC" --header "Authorization: Bearer $TOK" ${SID:+--header "Mcp-Session-Id: $SID"} --header "Mcp-Protocol-Version: 2025-06-18" --post-data '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' $U
head -1 /tmp/init.hdr
grep -o '"name":"ipfs_[a-z_]*"' /tmp/tools.out | sort -u

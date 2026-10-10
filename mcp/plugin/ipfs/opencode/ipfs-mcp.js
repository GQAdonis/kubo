// OpenCode plugin: registers the Kubo "ipfs mcp" stdio server and the IPFS
// skills. Both are added only when the user has not already configured them,
// so an existing `mcp.ipfs` entry or skills path always wins. Nothing here
// contacts a network endpoint; the server finds the user's node through its
// normal API discovery (--api, $IPFS_PATH/api).
import path from "node:path"
import { fileURLToPath } from "node:url"

// skills/ sits next to this directory inside the plugin root.
const skillsDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "skills")

export const IpfsMcpPlugin = async () => ({
  config: async (config) => {
    config.mcp = config.mcp ?? {}
    if (!config.mcp.ipfs) {
      config.mcp.ipfs = { type: "local", command: ["ipfs", "mcp"], enabled: true }
    }
    config.skills = config.skills ?? {}
    const paths = config.skills.paths ?? []
    if (!paths.includes(skillsDir)) {
      config.skills.paths = [...paths, skillsDir]
    }
  },
})

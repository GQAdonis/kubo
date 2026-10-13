import { CID } from 'multiformats/cid';

// A DNSLink name is an /ipns/ name that is a domain, not a key. Resolving one makes the
// node ask DNS for it, and an agent can choose the name, so the lookup can carry data out
// (security R13 H2). Names that are libp2p keys resolve without DNS.

const PEER_ID = /^(12D3KooW[1-9A-HJ-NP-Za-km-z]{44}|Qm[1-9A-HJ-NP-Za-km-z]{44})$/;
const LIBP2P_KEY_CODEC = 0x72;

/** True when `name` (no /ipns/ prefix, no sub-path) is a libp2p key, so resolving it needs no DNS. */
export function isKeyName(name: string): boolean {
  if (PEER_ID.test(name)) return true;
  try {
    return CID.parse(name).code === LIBP2P_KEY_CODEC;
  } catch {
    return false;
  }
}

/**
 * The tool arguments that name content or an IPNS name, per tool. Other string arguments
 * (MFS paths, key names) never reach name resolution.
 */
const NAME_FIELDS: Record<string, readonly string[]> = {
  ipfs_cat: ['path'],
  ipfs_get: ['path'],
  ipfs_ls: ['path'],
  pin_ls: ['path'],
  pin_add: ['path'],
  dag_get: ['path'],
  ipns_publish: ['path'],
  ipns_resolve: ['name'],
};

/** Does this value need a DNSLink lookup? Decided from the text alone, before any RPC call. */
function needsDnsLink(tool: string, field: string, value: string): boolean {
  let rest: string;
  if (tool === 'ipns_resolve' && field === 'name') {
    rest = value.startsWith('/ipns/') ? value.slice(6) : value;
  } else if (value.startsWith('/ipns/')) {
    rest = value.slice(6);
  } else {
    return false;
  }
  const first = rest.split('/')[0] ?? '';
  return !isKeyName(first);
}

/** The first argument of `tool` that would trigger a DNSLink lookup, if any. */
export function dnsLinkArgument(tool: string, input: Record<string, unknown>): string | undefined {
  for (const field of NAME_FIELDS[tool] ?? []) {
    const v = input[field];
    if (typeof v === 'string' && needsDnsLink(tool, field, v)) return field;
  }
  return undefined;
}

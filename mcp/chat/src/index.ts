// Entry point of @gqadonis/ipfs-agent-chat. Importing it defines <ipfs-agent-chat> and nothing else:
// React, the stylesheet and the fonts are in a chunk the element imports on first open.
import { defineIpfsAgentChat } from './element';

defineIpfsAgentChat();

export { ELEMENT_TAG, IpfsAgentChatElement, NOTICE_EVENT, OPEN_CHANGE_EVENT, defineIpfsAgentChat } from './element';
export type { ChatTheme } from './element';
export { SUGGESTED_AGUI_ENDPOINT } from './agui/endpoint';

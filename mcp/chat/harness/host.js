// The harness host page script. It plays the part of the Web UI fork's launcher: it loads the island
// module only when asked (island=closed|open), mounts the element, and hands it the endpoint and the
// bearer as a property. With island=none it does nothing, which gives the baseline page.
const config = JSON.parse(document.getElementById('harness-config').textContent);
// The config carries the bearer; a real launcher keeps it in memory only, so drop it from the markup.
document.getElementById('harness-config').remove();
// Test clock: the island reads Date.now to decide when a confirmation expires. Fixed, in page script, so it
// works the same in Chromium and Electron (Playwright's own clock also empties performance entries).
if (config.now) {
  const fixed = Date.parse(config.now);
  Date.now = () => fixed;
}
const launcher =document.getElementById('chat-launcher');
let element;

if (config.island !== 'none') {
  await import('/island/ipfs-agent-chat.js');
  element = document.createElement('ipfs-agent-chat');
  element.id = 'agent-chat';
  element.setAttribute('endpoint', config.endpoint);
  element.setAttribute('theme', config.theme);
  element.token = config.token; // property only; never an attribute
  document.body.append(element);
  element.addEventListener('ipfs-agent-chat-openchange', (event) => {
    launcher.setAttribute('aria-expanded', String(event.detail.open));
  });
  if (config.island === 'open') element.open = true;
}

launcher.addEventListener('click', () => element?.toggle());
window.__harnessReady = true;

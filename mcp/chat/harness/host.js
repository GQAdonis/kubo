// The harness host page script. It plays the part of the Web UI fork's launcher: it loads the island
// module only when asked (island=closed|open), mounts the element, and hands it the endpoint and the
// bearer as a property. With island=none it does nothing, which gives the baseline page.
const config = JSON.parse(document.getElementById('harness-config').textContent);
const launcher = document.getElementById('chat-launcher');
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

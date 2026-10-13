import { createRoot } from 'react-dom/client';
import { registerBundledFonts } from '../src/fonts/register';
import { UiScope } from '../src/scope/UiScope';
import { SCENES } from './scenes';

// Mounts the chat transcript scene inside an open shadow root, the way the Web UI fork mounts the
// <ipfs-agent-chat> island (ADR-0005). The host page (shadow.html) styles itself hostilely.
const params = new URLSearchParams(location.search);
const sceneId = params.get('scene') ?? 'chat-write-pending';
const theme = params.get('theme') === 'light' ? 'light' : 'dark';

async function start() {
  await registerBundledFonts();
  await document.fonts.ready;
  const host = document.getElementById('island-host');
  if (!host) throw new Error('missing #island-host');
  const shadow = host.attachShadow({ mode: 'open' });
  const mount = document.createElement('div');
  shadow.append(mount);
  const scene = SCENES.find((s) => s.id === sceneId);
  if (!scene) throw new Error(`unknown scene ${sceneId}`);
  createRoot(mount).render(
    <UiScope theme={theme} paint>
      <div style={{ padding: 16 }}>{scene.render()}</div>
    </UiScope>,
  );
  requestAnimationFrame(() => requestAnimationFrame(() => document.documentElement.setAttribute('data-ready', 'true')));
}

void start();

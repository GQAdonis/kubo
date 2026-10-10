import './gallery.css';
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { registerBundledFonts } from '../src/fonts/register';
import { UiScope, type UiTheme } from '../src/scope/UiScope';
import { SCENES } from './scenes';

// ?scene=<id>|all  &theme=dark|light|auto
const params = new URLSearchParams(location.search);
const sceneParam = params.get('scene') ?? 'all';
const theme = (['dark', 'light', 'auto'].includes(params.get('theme') ?? '') ? params.get('theme') : 'dark') as UiTheme;
const scenes = sceneParam === 'all' ? SCENES : SCENES.filter((s) => s.id === sceneParam);

function Gallery() {
  return (
    <UiScope theme={theme} paint>
      <main className="gallery-main">
        <h1 className="gallery-title">Component gallery{scenes.length === 1 ? `: ${scenes[0]?.title}` : ''}</h1>
        {scenes.map((s) => (
          <section key={s.id} aria-labelledby={`scene-${s.id}`} data-scene={s.id}>
            <h2 id={`scene-${s.id}`} className="gallery-scene-title">
              {s.title}
            </h2>
            {s.render()}
          </section>
        ))}
      </main>
    </UiScope>
  );
}

async function start() {
  await registerBundledFonts();
  await document.fonts.ready;
  const el = document.getElementById('root');
  if (!el) throw new Error('missing #root');
  createRoot(el).render(
    <StrictMode>
      <Gallery />
    </StrictMode>,
  );
  // Let layout and fonts settle, then tell the specs the page is stable.
  requestAnimationFrame(() => requestAnimationFrame(() => document.documentElement.setAttribute('data-ready', 'true')));
}

void start();

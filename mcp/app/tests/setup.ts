import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/react';
import { afterEach } from 'vitest';

afterEach(() => {
  cleanup();
});

// axe probes canvas for icon-font detection; jsdom has none. Report "no context" quietly.
HTMLCanvasElement.prototype.getContext = (() => null) as unknown as HTMLCanvasElement['getContext'];

// React 19 act environment for @testing-library/react.
(globalThis as Record<string, unknown>)['IS_REACT_ACT_ENVIRONMENT'] = true;

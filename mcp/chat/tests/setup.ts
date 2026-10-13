import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/react';
import { afterEach } from 'vitest';

// Some files run in the node environment (`// @vitest-environment node`): they have no DOM to clean up.
if (typeof document !== 'undefined') {
  afterEach(() => {
    cleanup();
  });

  // axe probes canvas for icon-font detection; jsdom has none. Report "no context" quietly.
  HTMLCanvasElement.prototype.getContext = (() => null) as unknown as HTMLCanvasElement['getContext'];
}

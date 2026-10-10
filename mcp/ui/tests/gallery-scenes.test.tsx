import { render } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { SCENES } from '../gallery/scenes';
import { UiScope } from '../src/scope/UiScope';
import { SCENE_IDS } from './visual/helpers';

describe('gallery scenes (the visual-regression inputs)', () => {
  it('match the scene list the Playwright specs iterate', () => {
    expect([...SCENE_IDS]).toEqual(SCENES.map((s) => s.id));
  });

  it('every scene renders from real fixtures without throwing', () => {
    for (const scene of SCENES) {
      const { container, unmount } = render(<UiScope injectStyles={false}>{scene.render()}</UiScope>);
      expect(container.textContent?.length, scene.id).toBeGreaterThan(20);
      unmount();
    }
  });
});

import { describe, expect, it } from 'vitest';
import { GESTURE_WINDOW_MS, createGestureGate, decisionOf } from '../../src/agui/gesture';

// The gate is the heart of G11-3. These tests drive it with plain objects so every branch is visible;
// tests/unit/interrupt-ui.test.tsx drives the same gate through the real card and real DOM events.

function button(kind: 'primary' | 'secondary'): HTMLButtonElement {
  document.body.innerHTML = `<section class="iu-approval"><div class="iu-approval__actions"><button class="iu-btn iu-btn--${kind}">x</button></div></section>`;
  return document.querySelector('button') as HTMLButtonElement;
}

describe('decisionOf', () => {
  it('reads the decision control from the card markup', () => {
    expect(decisionOf(button('primary'))).toBe('approve');
    expect(decisionOf(button('secondary'))).toBe('deny');
  });

  it('calls anything else other', () => {
    document.body.innerHTML = '<div class="iu-approval"><summary>Arguments</summary></div>';
    expect(decisionOf(document.querySelector('summary'))).toBe('other');
    expect(decisionOf(null)).toBe('other');
    expect(decisionOf(document.body)).toBe('other');
  });
});

describe('gesture gate', () => {
  it('accepts a trusted activation of the matching control, once', () => {
    const gate = createGestureGate(() => 0);
    gate.record({ isTrusted: true, target: button('primary') });
    expect(gate.consume('approve')).toBe(true);
    expect(gate.consume('approve')).toBe(false);
  });

  it('refuses an untrusted activation', () => {
    const gate = createGestureGate(() => 0);
    gate.record({ isTrusted: false, target: button('primary') });
    expect(gate.consume('approve')).toBe(false);
  });

  it('refuses when nothing was recorded: a script that calls the handler directly', () => {
    expect(createGestureGate(() => 0).consume('approve')).toBe(false);
    expect(createGestureGate(() => 0).consume('deny')).toBe(false);
  });

  it('refuses approve when the trusted activation was the deny control, and the other way round', () => {
    const gate = createGestureGate(() => 0);
    gate.record({ isTrusted: true, target: button('secondary') });
    expect(gate.consume('approve')).toBe(false);
    gate.record({ isTrusted: true, target: button('primary') });
    expect(gate.consume('deny')).toBe(false);
  });

  it('refuses a trusted click on some other part of the card followed by a scripted call', () => {
    const gate = createGestureGate(() => 0);
    document.body.innerHTML = '<div class="iu-approval"><summary>Arguments</summary></div>';
    gate.record({ isTrusted: true, target: document.querySelector('summary') });
    expect(gate.consume('approve')).toBe(false);
  });

  it('lets a later untrusted event overwrite an earlier trusted one', () => {
    const gate = createGestureGate(() => 0);
    const b = button('primary');
    gate.record({ isTrusted: true, target: b });
    gate.record({ isTrusted: false, target: b });
    expect(gate.consume('approve')).toBe(false);
  });

  it('refuses a stale activation', () => {
    let t = 0;
    const gate = createGestureGate(() => t);
    gate.record({ isTrusted: true, target: button('primary') });
    t = GESTURE_WINDOW_MS + 1;
    expect(gate.consume('approve')).toBe(false);
  });

  it('spends the record on a refused call too, so a retry needs a new gesture', () => {
    const gate = createGestureGate(() => 0);
    gate.record({ isTrusted: true, target: button('secondary') });
    expect(gate.consume('approve')).toBe(false);
    expect(gate.consume('deny')).toBe(false);
  });
});

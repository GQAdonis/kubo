// Text that arrives from the network is rendered as text, never as markup. These helpers also make
// characters that can spoof what the reader sees (bidirectional overrides, control characters)
// visible instead of acting on the display.

const BIDI = /[‪-‮⁦-⁩‎‏؜]/gu;
// C0 and C1 controls except tab and newline, plus DEL.
// eslint-disable-next-line no-control-regex
const CONTROL = /[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f-\u009f]/gu;

const hex = (cp: number) => `\\u{${cp.toString(16).toUpperCase()}}`;

/** Replace spoofing and control characters with a visible escape. Everything else is unchanged. */
export function displayText(input: string): string {
  return input.replace(BIDI, (c) => hex(c.codePointAt(0) ?? 0)).replace(CONTROL, (c) => hex(c.codePointAt(0) ?? 0));
}

export interface Clipped {
  readonly text: string;
  readonly clipped: boolean;
  /** Characters omitted from the end. */
  readonly omitted: number;
}

/** Cap what one preview renders so a large result cannot freeze the view. */
export function clip(input: string, max: number): Clipped {
  if (input.length <= max) return { text: input, clipped: false, omitted: 0 };
  return { text: input.slice(0, max), clipped: true, omitted: input.length - max };
}

export const PREVIEW_LIMIT = 64 * 1024;

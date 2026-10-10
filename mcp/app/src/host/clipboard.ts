/**
 * Copy text from inside a sandboxed iframe. navigator.clipboard.writeText needs a permission the host
 * may not grant (clipboard-write); a hidden textarea and execCommand("copy") works inside a user
 * gesture without one. Throws when neither works, so the button reports the failure in place.
 */
export async function copyText(text: string): Promise<void> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      return;
    }
  } catch {
    // Permission denied or no focus: fall through to the selection-based copy.
  }
  const area = document.createElement('textarea');
  area.value = text;
  area.setAttribute('readonly', '');
  area.setAttribute('aria-hidden', 'true');
  area.style.cssText = 'position:fixed;top:0;left:0;opacity:0;pointer-events:none';
  document.body.append(area);
  try {
    area.select();
    if (!document.execCommand('copy')) throw new Error('Copy was refused by the browser.');
  } finally {
    area.remove();
  }
}

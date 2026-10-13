/**
 * Register the bundled brand faces with document.fonts from embedded bytes.
 *
 * The MCP Apps default CSP has no font-src (it falls back to default-src 'none'), so a url() or data:
 * URL in @font-face can be refused. A FontFace built from an ArrayBuffer performs no fetch and is not
 * subject to font-src. document.fonts is document-wide, so the faces also reach shadow roots, where
 * @font-face rules are ignored. If FontFace is unavailable the font stacks fall back to system fonts.
 */
let registered: Promise<number> | null = null;

export function registerBundledFonts(doc: Document = document): Promise<number> {
  if (registered) return registered;
  registered = load(doc);
  return registered;
}

async function load(doc: Document): Promise<number> {
  const FontFaceCtor = (globalThis as { FontFace?: typeof FontFace }).FontFace;
  if (!FontFaceCtor || !doc.fonts) return 0;
  const { BUNDLED_FACES } = await import('./fonts-data');
  let added = 0;
  for (const face of BUNDLED_FACES) {
    try {
      const font = new FontFaceCtor(face.family, decodeDataUri(face.dataUri), {
        weight: face.weight,
        style: 'normal',
        display: 'swap',
      });
      doc.fonts.add(await font.load());
      added++;
    } catch {
      // A face that fails to decode leaves the fallback stack in place; nothing else depends on it.
    }
  }
  return added;
}

function decodeDataUri(uri: string): ArrayBuffer {
  const comma = uri.indexOf(',');
  const binary = atob(uri.slice(comma + 1));
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  return bytes.buffer;
}

/** Test seam: forget the memoized registration. */
export function resetFontRegistration(): void {
  registered = null;
}

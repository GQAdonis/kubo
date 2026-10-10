// The Content-Security-Policy written into every view document as a <meta http-equiv> tag.
//
// It allows no remote origin and no network at all: connect-src is none, there is no font-src (the
// brand faces are registered from embedded bytes through the FontFace API, which does not fetch), and
// img-src is data: only. Inline script and style are allowed because the document is one self-contained
// file; nothing else can execute. A host applies its own CSP on top of this one and the browser
// enforces both, so this policy can only narrow what the host allows.
export const CSP = [
  "default-src 'none'",
  "script-src 'unsafe-inline'",
  "style-src 'unsafe-inline'",
  'img-src data:',
  "connect-src 'none'",
  "frame-src 'none'",
  "object-src 'none'",
  "base-uri 'none'",
  "form-action 'none'",
].join('; ');

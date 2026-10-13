// Brand woff2 files embedded as data URIs (copied from the Promethity branding directory into
// mcp/ui/fonts). Loaded lazily by registerBundledFonts so hosts that never register fonts do not
// carry the bytes. Newsreader is not listed: the brand reserves it for a pull quote and these views
// have none (DESIGN.md).
import syne600 from '../../fonts/syne-latin-600-normal.woff2?inline';
import syne700 from '../../fonts/syne-latin-700-normal.woff2?inline';
import hanken400 from '../../fonts/hanken-grotesk-latin-400-normal.woff2?inline';
import hanken500 from '../../fonts/hanken-grotesk-latin-500-normal.woff2?inline';
import hanken600 from '../../fonts/hanken-grotesk-latin-600-normal.woff2?inline';
import hanken700 from '../../fonts/hanken-grotesk-latin-700-normal.woff2?inline';
import mono400 from '../../fonts/jetbrains-mono-latin-400-normal.woff2?inline';
import mono500 from '../../fonts/jetbrains-mono-latin-500-normal.woff2?inline';
import mono600 from '../../fonts/jetbrains-mono-latin-600-normal.woff2?inline';

export interface BundledFace {
  readonly family: string;
  readonly weight: string;
  readonly dataUri: string;
}

export const BUNDLED_FACES: readonly BundledFace[] = [
  { family: 'IPFS UI Syne', weight: '600', dataUri: syne600 },
  { family: 'IPFS UI Syne', weight: '700', dataUri: syne700 },
  { family: 'IPFS UI Hanken Grotesk', weight: '400', dataUri: hanken400 },
  { family: 'IPFS UI Hanken Grotesk', weight: '500', dataUri: hanken500 },
  { family: 'IPFS UI Hanken Grotesk', weight: '600', dataUri: hanken600 },
  { family: 'IPFS UI Hanken Grotesk', weight: '700', dataUri: hanken700 },
  { family: 'IPFS UI JetBrains Mono', weight: '400', dataUri: mono400 },
  { family: 'IPFS UI JetBrains Mono', weight: '500', dataUri: mono500 },
  { family: 'IPFS UI JetBrains Mono', weight: '600', dataUri: mono600 },
];

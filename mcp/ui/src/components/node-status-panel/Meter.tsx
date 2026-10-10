import { formatInt } from '../../lib/format';

export interface MeterProps {
  label: string;
  value: number;
  max: number;
  /** The value in words, shown beside the bar. The bar is never the only carrier of the number. */
  text: string;
}

const WARN_AT = 0.9;

/**
 * A flat measured fill. Drawn in SVG so the width is a presentation attribute: no inline style, which
 * keeps it working under a CSP that forbids style attributes.
 */
export function Meter({ label, value, max, text }: MeterProps) {
  const ratio = max > 0 ? Math.min(Math.max(value / max, 0), 1) : 0;
  const percent = Math.round(ratio * 1000) / 10;
  return (
    <div className="iu-meter">
      <div
        role="meter"
        aria-label={label}
        aria-valuemin={0}
        aria-valuemax={max}
        aria-valuenow={Math.min(value, max)}
        aria-valuetext={`${text}, ${formatInt(Math.round(percent))} percent`}
        className="iu-meter__track"
      >
        <svg viewBox="0 0 100 8" preserveAspectRatio="none" aria-hidden="true" focusable="false">
          <rect className={ratio >= WARN_AT ? 'iu-meter__fill iu-meter__fill--warn' : 'iu-meter__fill'} x="0" y="0" width={percent} height="8" />
        </svg>
      </div>
      <span className="iu-meter__text iu-num">
        {text}
        {ratio >= WARN_AT ? ' (nearly full)' : ''}
      </span>
    </div>
  );
}

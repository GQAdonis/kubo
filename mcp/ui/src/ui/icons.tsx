import {
  Check,
  ChevronDown,
  ChevronRight,
  CircleAlert,
  CircleCheck,
  CircleX,
  Clock,
  Copy,
  File,
  FileText,
  Folder,
  KeyRound,
  Link2,
  Network,
  Pin,
  type LucideProps,
} from 'lucide-react';

// Lucide (ISC) at the brand's 1.5px stroke. Decorative: every icon sits beside a visible word.
const base: LucideProps = { size: 16, strokeWidth: 1.5, 'aria-hidden': true, focusable: false };

export const IconCopy = (p: LucideProps) => <Copy {...base} {...p} />;
export const IconCheck = (p: LucideProps) => <Check {...base} {...p} />;
export const IconChevronRight = (p: LucideProps) => <ChevronRight {...base} {...p} />;
export const IconChevronDown = (p: LucideProps) => <ChevronDown {...base} {...p} />;
export const IconFolder = (p: LucideProps) => <Folder {...base} {...p} />;
export const IconFile = (p: LucideProps) => <File {...base} {...p} />;
export const IconFileText = (p: LucideProps) => <FileText {...base} {...p} />;
export const IconLink = (p: LucideProps) => <Link2 {...base} {...p} />;
export const IconClock = (p: LucideProps) => <Clock {...base} {...p} />;
export const IconAlert = (p: LucideProps) => <CircleAlert {...base} {...p} />;
export const IconOk = (p: LucideProps) => <CircleCheck {...base} {...p} />;
export const IconDenied = (p: LucideProps) => <CircleX {...base} {...p} />;
export const IconKey = (p: LucideProps) => <KeyRound {...base} {...p} />;
export const IconPin = (p: LucideProps) => <Pin {...base} {...p} />;
export const IconNetwork = (p: LucideProps) => <Network {...base} {...p} />;

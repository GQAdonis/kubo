import { clsx, type ClassValue } from 'clsx';

/**
 * shadcn's `cn` helper. shadcn pairs clsx with tailwind-merge to resolve conflicting utilities; this
 * island's class lists are written by hand and never conflict, so it keeps clsx alone and the roughly
 * 9 kB (gzip) that tailwind-merge adds to a bundle that loads in a host page is not paid.
 */
export function cn(...inputs: ClassValue[]): string {
  return clsx(inputs);
}

import { createContext, use, useMemo, type ComponentProps, type ReactNode } from 'react';
import { cn } from './cn';

// A right-side offcanvas sidebar in the shape of shadcn/ui's Sidebar (SidebarProvider, Sidebar,
// SidebarHeader, SidebarContent, SidebarFooter, SidebarTrigger), adapted for a shadow root:
//
//   - state is controlled by the host element, not by a cookie (shadcn persists `sidebar_state` in a
//     cookie; this island writes no cookie and no storage);
//   - there is no Sheet or portal: Radix portals attach to document.body, outside the shadow root,
//     where none of this CSS applies. The panel is a fixed-position <aside> inside the root;
//   - it is a non-modal complementary landmark, not a dialog: the host page stays usable beside it.
//
// The slide itself is plain CSS in src/styles/island.css (.ac-sidebar), keyed on data-state.

export interface SidebarContextValue {
  readonly open: boolean;
  readonly side: 'right';
  readonly setOpen: (open: boolean) => void;
}

const SidebarContext = createContext<SidebarContextValue | null>(null);

export function useSidebar(): SidebarContextValue {
  const ctx = use(SidebarContext);
  if (!ctx) throw new Error('useSidebar must be used inside <SidebarProvider>.');
  return ctx;
}

export interface SidebarProviderProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  children: ReactNode;
}

export function SidebarProvider({ open, onOpenChange, children }: SidebarProviderProps) {
  const value = useMemo<SidebarContextValue>(() => ({ open, side: 'right', setOpen: onOpenChange }), [open, onOpenChange]);
  return <SidebarContext value={value}>{children}</SidebarContext>;
}

export interface SidebarProps extends Omit<ComponentProps<'aside'>, 'aria-label'> {
  /** Accessible name of the landmark. */
  label: string;
}

export function Sidebar({ label, className, children, onKeyDown, ...rest }: SidebarProps) {
  const { open, side, setOpen } = useSidebar();
  return (
    <aside
      data-slot="sidebar"
      data-state={open ? 'open' : 'closed'}
      data-side={side}
      aria-label={label}
      // Closed means unreachable: no focus, no tab stop, not in the accessibility tree.
      inert={!open}
      className={cn('ac-sidebar', className)}
      onKeyDown={(event) => {
        onKeyDown?.(event);
        if (event.defaultPrevented) return;
        // Escape closes, except while an input method is composing text.
        if (event.key === 'Escape' && !event.nativeEvent.isComposing) {
          event.stopPropagation();
          setOpen(false);
        }
      }}
      {...rest}
    >
      {children}
    </aside>
  );
}

export function SidebarHeader({ className, ...rest }: ComponentProps<'header'>) {
  return <header data-slot="sidebar-header" className={cn('flex flex-col gap-s2 px-s3 pt-s3 pb-s2', className)} {...rest} />;
}

export function SidebarContent({ className, ...rest }: ComponentProps<'div'>) {
  return <div data-slot="sidebar-content" className={cn('flex min-h-0 flex-1 flex-col', className)} {...rest} />;
}

export function SidebarFooter({ className, ...rest }: ComponentProps<'footer'>) {
  return <footer data-slot="sidebar-footer" className={cn('flex flex-col gap-s2 bg-raised px-s3 py-s3', className)} {...rest} />;
}

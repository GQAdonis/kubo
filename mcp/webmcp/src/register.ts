import type { WebMcpTool } from './types.js';

/** The part of `document.modelContext` this package uses (WebMCP draft, 8 October 2026). */
export interface ModelContextLike {
  registerTool: (tool: WebMcpTool, options?: { signal?: AbortSignal }) => unknown;
}

export interface Registration {
  registered: string[];
  failed: Array<{ name: string; message: string }>;
  /** Removes every tool this call registered (aborts the registration signal). Safe to call twice. */
  unregister: () => void;
}

/**
 * The page-level object, or undefined when the browser has no WebMCP. Only
 * `document.modelContext` is read: Codex has no `navigator.modelContext` and Kimi marks
 * it deprecated.
 */
export function getModelContext(doc: unknown = typeof document === 'undefined' ? undefined : document): ModelContextLike | undefined {
  const mc = (doc as { modelContext?: unknown } | undefined)?.modelContext;
  return mc && typeof (mc as ModelContextLike).registerTool === 'function' ? (mc as ModelContextLike) : undefined;
}

/**
 * Register tools through `registerTool(tool, { signal })`. One failing tool does not stop
 * the rest. Returns what was registered so the page can show the real state.
 */
export async function registerWebMcpTools(modelContext: ModelContextLike, tools: readonly WebMcpTool[]): Promise<Registration> {
  const controller = new AbortController();
  const registered: string[] = [];
  const failed: Array<{ name: string; message: string }> = [];
  for (const tool of tools) {
    try {
      await modelContext.registerTool(tool, { signal: controller.signal });
      registered.push(tool.name);
    } catch (e) {
      failed.push({ name: tool.name, message: e instanceof Error ? e.message : 'registration failed' });
    }
  }
  return { registered, failed, unregister: () => controller.abort() };
}

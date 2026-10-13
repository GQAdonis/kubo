// Order matters: tokens first, then base, then components. Shared by the Vite
// build (dist/ipfs-ui.css), the UiScope runtime injection and the tests.
export const STYLE_FILES = ['tokens.css', 'base.css', 'components.css', 'views.css', 'chat.css'] as const;

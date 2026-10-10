// The one place the lazy half of the island is imported. A separate module so the element has a single
// seam to load through, and so a test can count loads: a page that never opens the sidebar must never
// reach this call.
export const loadIsland = () => import('./island/mount');

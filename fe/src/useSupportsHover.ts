import { useEffect, useState } from 'react';

// (hover: hover) is the standard way to ask "does this input mechanism
// support hovering at all" rather than guessing from viewport width, which
// conflates screen size with input type (a touch laptop at desktop width
// still has no real hover, and resizing a desktop window never changes
// this - a narrow window still has a real mouse).
const HOVER_QUERY = '(hover: hover) and (pointer: fine)';

// Extracted out of CardThumbnail.tsx (the original, single caller) so
// ModelViewer.tsx - a second caller with an unrelated reason to ask "is
// this a real desktop pointer, not a touchscreen" (keeping its own
// controls always visible rather than fading them, see
// CONTROLS_HIDE_DELAY_MS's own doc comment there) - can ask the same
// question without copying the matchMedia setup.
//
// Reactive rather than a one-time check at module load - a hybrid 2-in-1
// device (or a mouse plugged into/unplugged from an otherwise touch-only
// one) can change hover capability mid-session, and whichever behavior
// depends on this should follow that without needing a page reload.
// matchMedia's own change event is what actually fires when this specific
// capability flips - deliberately not a resize listener, which would fire
// for reasons unrelated to hover/pointer capability entirely.
export const useSupportsHover = () => {
  const [supportsHover, setSupportsHover] = useState(
    () =>
      typeof window !== 'undefined' && window.matchMedia(HOVER_QUERY).matches,
  );

  useEffect(() => {
    const mql = window.matchMedia(HOVER_QUERY);
    const onChange = (event: MediaQueryListEvent) =>
      setSupportsHover(event.matches);
    mql.addEventListener('change', onChange);
    return () => mql.removeEventListener('change', onChange);
  }, []);

  return supportsHover;
};

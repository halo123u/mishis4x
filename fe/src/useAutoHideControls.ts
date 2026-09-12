import { type RefObject, useEffect, useState } from 'react';

// How long a chrome overlay (ModelViewer's Back/Flip/Broadcast row,
// ModelDisplay's single Exit display mode button) stays visible with no
// interaction before fading out - long enough to read/react to a fresh
// screen, short enough that it's actually out of the way for someone
// just watching the model (or, for ModelDisplay specifically, watching
// the physical Pepper's Ghost reflection - a persistent button sitting
// in that reflection would be a constant reminder this is a screen, not
// the illusion the whole rig exists for). Matches the general "video
// player controls" convention (tap to bring the chrome back, it fades
// again on its own) rather than requiring a deliberate dismiss.
const CONTROLS_HIDE_DELAY_MS = 3000;

// Extracted out of ModelViewer.tsx (the original caller) so
// ModelDisplay.tsx's own single "Exit display mode" button (see its own
// doc comment) can fade the same way without copying this effect
// wholesale a second time.
//
// skip disables the fade entirely (controls always visible) rather than
// just lengthening the delay - ModelViewer uses this while loading/
// errored (someone stuck on a slow load or a failure needs an obvious,
// persistent way out, not one that fades away while they're waiting or
// reading the error) and on a real desktop pointer (see useSupportsHover
// - there's no cramped viewport to reclaim, and the *only* way to bring
// hidden controls back is a tap/click on the stage, which can have its
// own disruptive side effect, see ModelViewer's own doc comment on this).
export function useAutoHideControls(
  stageRef: RefObject<HTMLElement | null>,
  skip: boolean,
): boolean {
  const [controlsVisible, setControlsVisible] = useState(true);

  useEffect(() => {
    if (skip) {
      return;
    }

    let hideTimer: ReturnType<typeof setTimeout>;
    const resetHideTimer = () => {
      setControlsVisible(true);
      clearTimeout(hideTimer);
      hideTimer = setTimeout(
        () => setControlsVisible(false),
        CONTROLS_HIDE_DELAY_MS,
      );
    };

    const initialTimer = setTimeout(resetHideTimer, 0);

    // pointerdown, not click - fires uniformly for touch/mouse/pen, and
    // fires immediately on press rather than waiting for a full
    // click-cycle, matching how quickly the controls should reappear.
    const stage = stageRef.current;
    stage?.addEventListener('pointerdown', resetHideTimer);

    return () => {
      clearTimeout(initialTimer);
      clearTimeout(hideTimer);
      stage?.removeEventListener('pointerdown', resetHideTimer);
    };
  }, [stageRef, skip]);

  // skip: controlsVisible itself is never toggled off in that branch
  // above (the effect returns before ever calling setControlsVisible(
  // false)), but a caller flipping from skip=false to skip=true mid-fade
  // shouldn't have to wait out whatever hide timer was already ticking -
  // OR it in directly rather than relying on the next real interaction
  // to fix it.
  return controlsVisible || skip;
}

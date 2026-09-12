import { useEffect, useRef, useState } from 'react';
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { useModelCanvas } from '../useModelCanvas';
import styles from './ModelViewer.module.css';

// How long the back button stays visible with no interaction before
// fading out - long enough to read/react to a fresh screen, short
// enough that it's actually out of the way for someone just watching
// the model. Matches the general "video player controls" convention
// (tap to bring the chrome back, it fades again on its own) rather than
// requiring a deliberate dismiss.
const CONTROLS_HIDE_DELAY_MS = 3000;

// Maps a flip mode to the CSS transform that produces it - a plain
// lookup rather than building the transform string directly, so an
// unrecognized value (bad query param, corrupted localStorage) falls
// back to no transform at all instead of silently passing junk through
// to the stylesheet. Order here is the cycle order the toggle button
// steps through.
const FLIP_MODES = ['x', 'y', '180'] as const;
type FlipMode = (typeof FLIP_MODES)[number];
const FLIP_TRANSFORMS: Record<FlipMode, string> = {
  x: 'scaleX(-1)',
  y: 'scaleY(-1)',
  '180': 'rotate(180deg)',
};

// Scoped to this feature specifically (not a generic app-wide settings
// key) since it's a physical-device quirk, not a user preference this
// app has any other concept of.
const FLIP_MODE_STORAGE_KEY = 'modelViewerFlipMode';
const PEPPER_MODE_STORAGE_KEY = 'modelViewerPepperMode';
const BROADCAST_MODE_STORAGE_KEY = 'modelViewerBroadcastMode';

const ModelViewer = () => {
  const { charCode } = useParams<{ charCode: string }>();
  const navigate = useNavigate();
  const stageRef = useRef<HTMLDivElement>(null);
  const [searchParams, setSearchParams] = useSearchParams();
  // Physical Pepper's Ghost/acrylic-reflection rigs need the rendered
  // character mirrored/rotated to come out correctly oriented once the
  // reflection itself flips it again - not something a normal browser
  // viewer of this page ever wants, so it defaults off. Settable two
  // ways, kept in sync with each other: the ?flip=x|y|180 query param
  // (handy for the "try it against the real acrylic" workflow, where
  // editing a URL and reloading beats a redeploy per attempt), and the
  // flipToggle button below it for a device that's actually mounted
  // behind the acrylic, where there's no practical way to type a URL -
  // see that button's own doc comment. localStorage is what makes the
  // button's choice stick across navigating to a different character on
  // the same physical device, without needing the URL's own help.
  const [flipMode, setFlipMode] = useState<FlipMode | null>(() => {
    const fromQuery = searchParams.get('flip');
    if (fromQuery && (FLIP_MODES as readonly string[]).includes(fromQuery)) {
      return fromQuery as FlipMode;
    }
    const stored = localStorage.getItem(FLIP_MODE_STORAGE_KEY);
    return stored && (FLIP_MODES as readonly string[]).includes(stored)
      ? (stored as FlipMode)
      : null;
  });
  const flipTransform = flipMode ? FLIP_TRANSFORMS[flipMode] : undefined;
  // Pepper Mode is a second, independent concern from flipMode above -
  // flipMode is about the character looking right to the *audience*
  // seeing it through the acrylic's reflection; this is about whoever's
  // physically operating the device being able to read/tap Back and the
  // flip toggle at all. On a rig where the phone itself ends up mounted
  // rotated 180° relative to the operator (the same mounting that makes
  // the reflection work in the first place), the controls - normally
  // bottom-of-screen, right-side-up - land upside-down for them. Pepper
  // Mode counter-rotates the whole .controls cluster 180° in place (see
  // .controlsPepperMode's own doc comment for why this doesn't also
  // reposition it - confirmed against the real rig that the anchor
  // point should stay put) so the labels read correctly again despite
  // the phone's own physical rotation. Same dual-source persistence as
  // flipMode: ?pepper=1 for the initial setup workflow, localStorage
  // (PEPPER_MODE_STORAGE_KEY) for the on-screen toggle to stick across
  // characters without needing the URL's help again.
  const [pepperMode, setPepperMode] = useState<boolean>(() => {
    const fromQuery = searchParams.get('pepper');
    if (fromQuery !== null) {
      return fromQuery === '1' || fromQuery === 'true';
    }
    return localStorage.getItem(PEPPER_MODE_STORAGE_KEY) === '1';
  });
  // Broadcast is a third, independent concern again - flipMode/pepperMode
  // are both about *this* browser's own rendering/controls; this is
  // about turning this same page into a remote control for a second,
  // separate device (see ModelDisplay.tsx) - a phone mounted inside a
  // physical rig with no practical way to interact with it directly, the
  // same reasoning ?flip=/?pepper= exist for at all. When on, every
  // character/flip/pepper combination this browser lands on is also
  // pushed to the shared display state (see the broadcast effect below)
  // - off by default so ordinary browsing (or a second person just
  // looking at a character) never disturbs a live display by accident.
  // Same dual-source persistence pattern as the other two.
  const [broadcastMode, setBroadcastMode] = useState<boolean>(() => {
    const fromQuery = searchParams.get('broadcast');
    if (fromQuery !== null) {
      return fromQuery === '1' || fromQuery === 'true';
    }
    return localStorage.getItem(BROADCAST_MODE_STORAGE_KEY) === '1';
  });
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const { loading, error } = useModelCanvas(charCode, canvasRef);
  const [controlsVisible, setControlsVisible] = useState(true);

  // Steps null -> 'x' -> 'y' -> '180' -> null - persisted to
  // localStorage (survives navigating to a different character on this
  // same device/browser) and mirrored into the URL (survives a refresh,
  // and keeps a manually-edited ?flip= URL and the button from fighting
  // each other over which one's "right").
  const cycleFlipMode = () => {
    const next =
      flipMode === null
        ? FLIP_MODES[0]
        : (FLIP_MODES[FLIP_MODES.indexOf(flipMode) + 1] ?? null);
    setFlipMode(next);
    if (next) {
      localStorage.setItem(FLIP_MODE_STORAGE_KEY, next);
    } else {
      localStorage.removeItem(FLIP_MODE_STORAGE_KEY);
    }
    setSearchParams(
      (prev) => {
        const updated = new URLSearchParams(prev);
        if (next) {
          updated.set('flip', next);
        } else {
          updated.delete('flip');
        }
        return updated;
      },
      { replace: true },
    );
  };

  const togglePepperMode = () => {
    const next = !pepperMode;
    setPepperMode(next);
    if (next) {
      localStorage.setItem(PEPPER_MODE_STORAGE_KEY, '1');
    } else {
      localStorage.removeItem(PEPPER_MODE_STORAGE_KEY);
    }
    setSearchParams(
      (prev) => {
        const updated = new URLSearchParams(prev);
        if (next) {
          updated.set('pepper', '1');
        } else {
          updated.delete('pepper');
        }
        return updated;
      },
      { replace: true },
    );
  };

  const toggleBroadcastMode = () => {
    const next = !broadcastMode;
    setBroadcastMode(next);
    if (next) {
      localStorage.setItem(BROADCAST_MODE_STORAGE_KEY, '1');
    } else {
      localStorage.removeItem(BROADCAST_MODE_STORAGE_KEY);
    }
    setSearchParams(
      (prev) => {
        const updated = new URLSearchParams(prev);
        if (next) {
          updated.set('broadcast', '1');
        } else {
          updated.delete('broadcast');
        }
        return updated;
      },
      { replace: true },
    );
  };

  // Pushes this browser's current character/flip/pepper to the shared
  // display state (see ModelDisplay.tsx, which polls the same endpoint)
  // whenever any of them change, but only while broadcastMode is on -
  // see its own doc comment above for why that's opt-in. Deliberately
  // doesn't also clear the display when broadcastMode turns off: turning
  // this browser's remote off shouldn't blank whatever's still live,
  // same as unplugging a TV remote doesn't turn the TV off.
  useEffect(() => {
    if (!broadcastMode || !charCode) {
      return;
    }
    void fetch('/api/models/display', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        char_code: charCode,
        flip: flipMode ?? '',
        pepper: pepperMode,
      }),
    });
  }, [broadcastMode, charCode, flipMode, pepperMode]);

  // Auto-hides the back button after a stretch of no interaction, and
  // brings it back on any tap/click anywhere in the stage (including one
  // that also triggers the character's own tap-to-react motion+audio -
  // there's no need to distinguish "tapped the character" from "tapped
  // to bring controls back", both are true at once). A separate effect
  // from the Spine setup (see useModelCanvas): this is pure UI chrome
  // state, unrelated to loading/rendering the model itself, and touching
  // it doesn't need to re-run any of that expensive setup.
  //
  // Deliberately not auto-hidden while loading or errored - someone
  // stuck on a slow load or a failure needs an obvious, persistent way
  // out, not one that fades away while they're waiting or reading the
  // error.
  useEffect(() => {
    // Both branches below only ever call setControlsVisible from a
    // deferred callback (setTimeout(..., 0), a real event listener), not
    // synchronously from the effect body itself - react-hooks/
    // set-state-in-effect flags the latter as a cascading-render smell,
    // same reasoning as SetDetail.tsx's own setTimeout(..., 0) around
    // setHighlightedCardID.
    if (loading || error) {
      const showTimer = setTimeout(() => setControlsVisible(true), 0);
      return () => clearTimeout(showTimer);
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
  }, [loading, error]);

  return (
    <div className={styles.stage} ref={stageRef}>
      <div
        className={[
          styles.controls,
          controlsVisible ? '' : styles.controlsHidden,
          pepperMode ? styles.controlsPepperMode : '',
        ]
          .filter(Boolean)
          .join(' ')}
      >
        {/* Real browser back, not a fixed /models link: this route is
            reachable both from the model picker (/models) and directly
            from a linked card in the collection tracker (CardModelLink,
            SetDetail.tsx) - going back should return to wherever the
            user actually came from, not always the picker. Bottom, not
            top-left: top corners are the hardest part of a phone screen
            to reach one-handed, exactly where this sat before - see
            CONTROLS_HIDE_DELAY_MS above for why it also isn't
            permanently on-screen once it's actually reachable. */}
        <button
          type="button"
          onClick={() => navigate(-1)}
          className={styles.back}
        >
          ← Back
        </button>
        {/* Same flip a ?flip= URL param already does (see FLIP_MODES'
            own doc comment) - this is the on-screen way to reach it for
            a device actually mounted behind a physical acrylic/Pepper's
            Ghost reflector, where there's no address bar to type a URL
            into at all, unlike a phone/desktop browser just testing the
            page normally. */}
        <button
          type="button"
          onClick={cycleFlipMode}
          className={
            flipMode
              ? `${styles.flipToggle} ${styles.flipToggleActive}`
              : styles.flipToggle
          }
          aria-label="Cycle the display flip mode, for a physical Pepper's Ghost/acrylic-reflection setup"
        >
          Flip: {flipMode ? flipMode.toUpperCase() : 'Off'}
        </button>
        {/* See pepperMode's own doc comment above - this rotates/
            repositions this whole button row (itself included), not
            just the character, so toggling it back off is still
            readable/reachable the same way toggling it on was. */}
        <button
          type="button"
          onClick={togglePepperMode}
          className={
            pepperMode
              ? `${styles.flipToggle} ${styles.flipToggleActive}`
              : styles.flipToggle
          }
          aria-label="Toggle Pepper Mode - rotates and repositions these controls for reading/tapping them on a device physically mounted upside-down for a Pepper's Ghost rig"
        >
          Pepper: {pepperMode ? 'On' : 'Off'}
        </button>
        {/* See broadcastMode's own doc comment above - turns this
            browser into a remote control for a second device (see
            ModelDisplay.tsx) instead of affecting anything about this
            page's own display. */}
        <button
          type="button"
          onClick={toggleBroadcastMode}
          className={
            broadcastMode
              ? `${styles.flipToggle} ${styles.flipToggleActive}`
              : styles.flipToggle
          }
          aria-label="Toggle broadcasting this character to the remote display"
        >
          Broadcast: {broadcastMode ? 'On' : 'Off'}
        </button>
      </div>
      {loading && !error && <p className={styles.status}>Loading…</p>}
      {error && <p className={styles.status}>{error}</p>}
      <canvas
        ref={canvasRef}
        className={styles.canvas}
        style={flipTransform ? { transform: flipTransform } : undefined}
      />
    </div>
  );
};

export default ModelViewer;

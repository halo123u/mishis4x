import { useEffect, useRef, useState } from 'react';
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { useModelCanvas } from '../useModelCanvas';
import { ModelDisplayStatus } from '../types';
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

// How long "Sent!"/an error stays on the Set as display button before
// it reverts to its normal label - long enough to register as
// confirmation, short enough that it's obviously not a persistent mode
// (see setDisplayStatus's own doc comment for why this replaced an
// earlier always-on Broadcast toggle).
const SET_DISPLAY_STATUS_RESET_MS = 1500;

// How often to check GET /api/models/display/status while this page is
// open - doesn't need to match ModelDisplay.tsx's own 1.5s render-
// driving poll exactly, just often enough that the Set as display
// button's enabled state feels current.
const DISPLAY_STATUS_POLL_INTERVAL_MS = 3000;

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
  // Pepper's Ghost rigs used to also need a "Pepper Mode" here that
  // counter-rotated this page's own on-screen controls 180° for whoever
  // was physically holding the trapped phone, reading them upside-down.
  // Now that ModelDisplay.tsx is a pure passive display with no buttons
  // of its own - every control lives here, on the controller - nothing
  // ever needs to read/tap this page's controls while mounted rotated
  // behind the acrylic, so that mode (and its whole rotate-in-place
  // rationale) no longer applies to anything. Removed rather than kept
  // around unused.
  const flipTransform = flipMode ? FLIP_TRANSFORMS[flipMode] : undefined;
  // 'idle' | 'sending' | 'sent' | 'error' - purely local UI feedback for
  // the Set as display button below, not a mode or a setting: an
  // earlier version of this had an always-on "Broadcast" toggle that
  // pushed every character/flip this browser landed on to the
  // shared display state (see ModelDisplay.tsx) continuously - replaced
  // because it meant remembering whether it was still on, and ordinary
  // browsing (or someone else just looking at a character) could
  // silently disturb a live display. This is a one-shot action instead:
  // nothing gets sent until this specific button is clicked, sending
  // exactly the character/flip on screen at that moment.
  const [setDisplayStatus, setSetDisplayStatus] = useState<
    'idle' | 'sending' | 'sent' | 'error'
  >('idle');
  // Same shape/reasoning as setDisplayStatus, for the separate "Trigger
  // touch" button below - see triggerTouch's own doc comment.
  const [triggerStatus, setTriggerStatus] = useState<
    'idle' | 'sending' | 'sent' | 'error'
  >('idle');
  // Whether a display is currently believed to be polling (see
  // be/handlers/model_display.go's ModelDisplay.Connected) - gates the
  // Set as display button below so it's not offering to send to a
  // display that's already gone.
  const [displayConnected, setDisplayConnected] = useState(false);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [controlsVisible, setControlsVisible] = useState(true);

  // Steps null -> 'x' -> 'y' -> '180' -> null - persisted to
  // localStorage (survives navigating to a different character on this
  // same device/browser) and mirrored into the URL (survives a refresh,
  // and keeps a manually-edited ?flip= URL and the button from fighting
  // each other over which one's "right").
  //
  // Also live-pushes to the shared display state when one's connected
  // (fire-and-forget, no loading/error UI of its own - this is a
  // continuous live-tweak, not the one-shot "Sent!"/"Could not send"
  // action setAsDisplay below already owns) so dialing in orientation
  // against the real acrylic is see-the-effect-immediately on whichever
  // screen the audience is actually looking at, not "adjust here, then
  // remember to re-click Set as display after every nudge."
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
    if (displayConnected) {
      fetch('/api/models/display/flip', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ flip: next ?? '' }),
      }).catch(() => {
        // Same tolerance as the status poll below - a transient failure
        // here just means the display keeps showing its last-known
        // orientation until the next successful push, not worth its own
        // error UI for a live-tweak control.
      });
    }
  };

  // Sends exactly the character/flip on screen right now to the shared
  // display state (see ModelDisplay.tsx, which polls the same endpoint)
  // - a single PUT, not a persisted mode. See setDisplayStatus's own doc
  // comment for why this replaced an always-on Broadcast toggle.
  const setAsDisplay = () => {
    if (!charCode) {
      return;
    }
    setSetDisplayStatus('sending');
    fetch('/api/models/display', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        char_code: charCode,
        flip: flipMode ?? '',
      }),
    })
      .then((res) => setSetDisplayStatus(res.ok ? 'sent' : 'error'))
      .catch(() => setSetDisplayStatus('error'));
  };

  // Remotely fires the display's own tap-to-motion+audio reaction (see
  // ModelDisplay.tsx's canvas.click() call) - a plain POST that bumps a
  // counter server-side (api.ModelDisplayState.Trigger), nothing about
  // this browser's own character/flip involved at all.
  const triggerTouch = () => {
    setTriggerStatus('sending');
    fetch('/api/models/display/trigger', { method: 'POST' })
      .then((res) => setTriggerStatus(res.ok ? 'sent' : 'error'))
      .catch(() => setTriggerStatus('error'));
  };

  // localReaction: false when a display is connected - tapping the
  // character on this page (the controller) is then remote control, not
  // a local preview, so the reaction (motion+audio) should happen on
  // whichever screen the audience is actually watching, not silently
  // also/instead fire here where nobody but the operator can see or hear
  // it. onTap fires triggerTouch every time regardless of
  // localReaction's value, so the same tap that would have played
  // locally with no display connected now drives the display's own
  // reaction instead. See useModelCanvas's own onTap/localReaction doc
  // comment.
  const { loading, error } = useModelCanvas(charCode, canvasRef, {
    onTap: () => {
      if (displayConnected) {
        triggerTouch();
      }
    },
    localReaction: !displayConnected,
  });

  // Reverts the button's transient "Sent!"/"Error" label back to normal
  // after a beat - a separate effect rather than a setTimeout inside
  // setAsDisplay itself, so a rapid second click cleanly restarts this
  // timer instead of an earlier one firing mid-flight and undoing the
  // newer status.
  useEffect(() => {
    if (setDisplayStatus === 'idle' || setDisplayStatus === 'sending') {
      return;
    }
    const timer = setTimeout(
      () => setSetDisplayStatus('idle'),
      SET_DISPLAY_STATUS_RESET_MS,
    );
    return () => clearTimeout(timer);
  }, [setDisplayStatus]);

  // Same reset-after-a-beat behavior as setDisplayStatus's own effect
  // above, for triggerStatus/Trigger touch instead.
  useEffect(() => {
    if (triggerStatus === 'idle' || triggerStatus === 'sending') {
      return;
    }
    const timer = setTimeout(
      () => setTriggerStatus('idle'),
      SET_DISPLAY_STATUS_RESET_MS,
    );
    return () => clearTimeout(timer);
  }, [triggerStatus]);

  // Keeps displayConnected current for as long as this page is open -
  // GET .../display/status, not GET .../display itself, so this check
  // never counts as proof the display is alive (see
  // ModelDisplay.Connected's own doc comment on the backend).
  // cache: 'no-store' - this must reflect this exact moment's real
  // connectivity every poll, confirmed directly that a default fetch()
  // can otherwise get served a stale cached response instead of
  // actually hitting the network again.
  useEffect(() => {
    let cancelled = false;

    const poll = () => {
      fetch('/api/models/display/status', { cache: 'no-store' })
        .then(async (res) => {
          if (res.status !== 200 || cancelled) {
            return;
          }
          const status: ModelDisplayStatus = await res.json();
          setDisplayConnected(status.connected);
        })
        .catch(() => {
          // A transient network hiccup shouldn't flip the button off -
          // just try again next tick, same tolerance as ModelDisplay.tsx's
          // own poll loop.
        });
    };

    poll();
    const interval = setInterval(poll, DISPLAY_STATUS_POLL_INTERVAL_MS);

    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, []);

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
            page normally. When a display is connected this also live-
            pushes to it (see cycleFlipMode's own doc comment) - the
            button's own label/highlight still reflects this browser's
            local flipMode either way, since that's the value actually
            being sent. */}
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
        {/* See setDisplayStatus's own doc comment above - a one-shot
            push of exactly this character/flip to the remote display
            (see ModelDisplay.tsx), not a persistent mode. */}
        <button
          type="button"
          onClick={setAsDisplay}
          disabled={setDisplayStatus === 'sending' || !displayConnected}
          className={
            setDisplayStatus === 'sent'
              ? `${styles.flipToggle} ${styles.flipToggleActive}`
              : styles.flipToggle
          }
          aria-label="Set this character as the remote display's current character"
        >
          {setDisplayStatus === 'sending' && 'Sending…'}
          {setDisplayStatus === 'sent' && 'Sent!'}
          {setDisplayStatus === 'error' && 'Could not send'}
          {setDisplayStatus === 'idle' &&
            (displayConnected ? 'Set as display' : 'No display connected')}
        </button>
        {/* See triggerTouch's own doc comment - fires the display's
            tap-to-motion+audio reaction remotely, same connectivity
            gate as Set as display for the same reason. */}
        <button
          type="button"
          onClick={triggerTouch}
          disabled={triggerStatus === 'sending' || !displayConnected}
          className={
            triggerStatus === 'sent'
              ? `${styles.flipToggle} ${styles.flipToggleActive}`
              : styles.flipToggle
          }
          aria-label="Trigger the remote display's tap-to-react motion and voice line"
        >
          {triggerStatus === 'sending' && 'Sending…'}
          {triggerStatus === 'sent' && 'Sent!'}
          {triggerStatus === 'error' && 'Could not send'}
          {triggerStatus === 'idle' && 'Trigger touch'}
        </button>
      </div>
      {loading && !error && <p className={styles.status}>Loading…</p>}
      {error && <p className={styles.status}>{error}</p>}
      {/* key={charCode}: forces a brand-new <canvas> DOM node per
          character rather than reusing one across a charCode change -
          see useModelCanvas's own cleanup, which explicitly calls
          WEBGL_lose_context's loseContext() to free GPU memory when
          leaving a model. Once a context is lost that way, the same
          canvas element can't just get a fresh working one back later
          (it stays lost until the browser explicitly restores it, which
          nothing here ever triggers) - reusing the element across a
          charCode change would render into a permanently dead context,
          a blank canvas. This route normally gets a full page remount
          on navigation anyway (so this rarely mattered before), but
          ModelDisplay.tsx's poll-driven charCode changes stay mounted
          continuously - confirmed live that a blank screen on every
          broadcast after the first was exactly this. */}
      <canvas
        key={charCode}
        ref={canvasRef}
        className={styles.canvas}
        // Suppressed while a display is connected: flipMode here exists
        // to correct how the character looks to whoever's watching the
        // physical acrylic reflection, and once that's the phone's job
        // instead of this browser's, this canvas rendering flipped too
        // would just be wrong for an ordinary controller screen - see
        // "external controls should do nothing on the browser when
        // there's a display" in this feature's design notes. The toggle
        // button above still cycles/sends flipMode regardless; only its
        // effect on *this* canvas is held back.
        style={
          flipTransform && !displayConnected
            ? { transform: flipTransform }
            : undefined
        }
      />
    </div>
  );
};

export default ModelViewer;

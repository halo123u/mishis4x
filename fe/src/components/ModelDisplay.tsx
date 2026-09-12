import { useEffect, useRef, useState } from 'react';
import { useModelCanvas } from '../useModelCanvas';
import { ModelDisplayState } from '../types';
import styles from './ModelDisplay.module.css';

// How often to poll GET /api/models/display for a change - see this
// component's own doc comment for why polling rather than a
// websocket/SSE push. Character switching in a physical art piece isn't
// a twitch-reflex control; a second or so of latency between a
// controller's tap and the display updating is a non-issue, and this
// keeps the whole feature to a plain fetch loop instead of new realtime
// infrastructure this app doesn't have anywhere else yet.
const POLL_INTERVAL_MS = 1500;

// The gap between the character's feet and the bottom edge of the
// screen on the real physical rig - see useModelCanvas's own
// verticalAlign/bottomMarginPx doc comment for why this exists at all.
// Confirmed against the real device: centering the character (this
// component's original behavior) left it sitting noticeably too high
// for the mounting angle/available headroom, forcing an awkward
// posture to compensate. A plain constant, not configurable from
// anywhere - this corrects one specific physical rig, not a general
// user-facing setting.
const BOTTOM_MARGIN_PX = 15;

const FLIP_TRANSFORMS: Record<string, string> = {
  x: 'scaleX(-1)',
  y: 'scaleY(-1)',
  '180': 'rotate(180deg)',
};

// The passive half of the model viewer's remote-control feature - a
// phone stuck inside a physical Pepper's Ghost/acrylic rig, with no
// practical way to interact with it directly, loads this page once and
// leaves it open. It has no buttons of its own (Back/Flip/Trigger touch
// all belong to ModelViewer.tsx, the interactive/controller side) - it
// just polls the shared display state and renders whatever character/
// flip combination is currently set, using the exact same Spine
// rendering logic as ModelViewer via useModelCanvas.
//
// Deliberately still tap-reactive (useModelCanvas's own tap-to-motion+
// audio isn't disabled here) - someone reaching directly into the rig
// to tap the character themselves is a harmless bonus, not something
// worth extra code to prevent.
const ModelDisplay = () => {
  const [charCode, setCharCode] = useState<string | null>(null);
  const [flip, setFlip] = useState<string>('');
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const { loading, error } = useModelCanvas(charCode ?? undefined, canvasRef, {
    verticalAlign: 'bottom',
    bottomMarginPx: BOTTOM_MARGIN_PX,
  });
  // null means "haven't seen a real poll response yet" - the first
  // successful poll just establishes this baseline rather than firing a
  // reaction, so a Trigger value left over from before this page loaded
  // (e.g. someone tapped the controller, then this display reconnected
  // later) doesn't replay as a phantom touch the moment it connects.
  // Independent of charCode: Trigger lives on the same shared state but
  // means something unrelated to which character is showing, so a
  // character change never resets this.
  const lastTriggerRef = useRef<number | null>(null);

  useEffect(() => {
    let cancelled = false;

    const poll = () => {
      // cache: 'no-store' - this must reflect the real current state
      // every poll (that's the entire point of a display that stays
      // mounted and re-polls), confirmed directly that a default
      // fetch() can otherwise get served a stale cached response
      // instead of actually hitting the network again.
      fetch('/api/models/display', { cache: 'no-store' })
        .then(async (res) => {
          if (res.status !== 200 || cancelled) {
            return;
          }
          const state: ModelDisplayState = await res.json();
          setCharCode(state.char_code || null);
          setFlip(state.flip ?? '');

          // canvas.click() - a real DOM method, not a synthetic input
          // event - fires the exact same 'click' listener
          // useModelCanvas's own tap-to-motion+audio reaction is
          // already attached to, so a remote "Trigger touch" behaves
          // identically to someone actually touching this screen.
          if (lastTriggerRef.current === null) {
            lastTriggerRef.current = state.trigger;
          } else if (state.trigger !== lastTriggerRef.current) {
            lastTriggerRef.current = state.trigger;
            canvasRef.current?.click();
          }
        })
        .catch(() => {
          // A transient network hiccup shouldn't blank whatever's
          // already showing - just try again next tick, same as any
          // other tolerant polling loop in this app.
        });
    };

    poll();
    const interval = setInterval(poll, POLL_INTERVAL_MS);

    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, []);

  const flipTransform = flip ? FLIP_TRANSFORMS[flip] : undefined;

  return (
    <div className={styles.stage}>
      {!charCode && !error && (
        <p className={styles.status}>Waiting for a character…</p>
      )}
      {charCode && loading && !error && (
        <p className={styles.status}>Loading…</p>
      )}
      {error && <p className={styles.status}>{error}</p>}
      {/* key={charCode} - see ModelViewer.tsx's identical comment on its
          own <canvas>: without this, switching characters here (this
          page's whole reason to exist - it stays mounted continuously,
          polling for a new charCode) renders into a canvas whose WebGL
          context useModelCanvas's own cleanup already explicitly killed
          for the *previous* character, producing a blank screen on
          every broadcast after the first. */}
      <canvas
        key={charCode}
        ref={canvasRef}
        className={styles.canvas}
        style={flipTransform ? { transform: flipTransform } : undefined}
      />
    </div>
  );
};

export default ModelDisplay;

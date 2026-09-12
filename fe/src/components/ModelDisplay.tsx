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

const FLIP_TRANSFORMS: Record<string, string> = {
  x: 'scaleX(-1)',
  y: 'scaleY(-1)',
  '180': 'rotate(180deg)',
};

// The passive half of the model viewer's remote-control feature - a
// phone stuck inside a physical Pepper's Ghost/acrylic rig, with no
// practical way to interact with it directly, loads this page once and
// leaves it open. It has no buttons of its own (Back/Flip/Pepper all
// belong to ModelViewer.tsx, the interactive/controller side) - it just
// polls the shared display state and renders whatever character/flip
// combination is currently set, using the exact same Spine rendering
// logic as ModelViewer via useModelCanvas.
//
// Deliberately still tap-reactive (useModelCanvas's own tap-to-motion+
// audio isn't disabled here) - someone reaching directly into the rig
// to tap the character themselves is a harmless bonus, not something
// worth extra code to prevent.
const ModelDisplay = () => {
  const [charCode, setCharCode] = useState<string | null>(null);
  const [flip, setFlip] = useState<string>('');
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const { loading, error } = useModelCanvas(charCode ?? undefined, canvasRef);

  useEffect(() => {
    let cancelled = false;

    const poll = () => {
      fetch('/api/models/display')
        .then(async (res) => {
          if (res.status !== 200 || cancelled) {
            return;
          }
          const state: ModelDisplayState = await res.json();
          setCharCode(state.char_code || null);
          setFlip(state.flip ?? '');
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
      <canvas
        ref={canvasRef}
        className={styles.canvas}
        style={flipTransform ? { transform: flipTransform } : undefined}
      />
    </div>
  );
};

export default ModelDisplay;

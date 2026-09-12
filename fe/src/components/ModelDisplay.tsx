import { useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useModelCanvas } from '../useModelCanvas';
import { useAutoHideControls } from '../useAutoHideControls';
import { ModelDisplayState } from '../types';
import styles from './ModelDisplay.module.css';

// How long to wait before retrying the WebSocket connection after it
// closes for any reason (the server restarting, a network hiccup, a
// phone backgrounding and killing the socket) - short enough that a
// reconnect feels instant on a physical rig nobody's actively watching
// for a dropped connection, not so short that a server that's genuinely
// down gets hammered with reconnect attempts. A fixed delay, not
// exponential backoff: this is one device reconnecting to a single-
// instance personal server, not a fleet of clients that could pile on
// load - there's no real backoff problem to solve here.
const RECONNECT_DELAY_MS = 1000;

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
// leaves it open. It holds one WebSocket connection open to the server
// for as long as the page is loaded, gets pushed the shared display
// state the instant a controller changes anything (see be/handlers/
// model_display.go's ServeModelDisplayWS), and renders whatever
// character/flip/pan/zoom combination that is, using the exact same
// Spine rendering logic as ModelViewer via useModelCanvas. Used to poll
// GET /api/models/display every 1.5s instead - replaced because that
// meant up to a full poll interval of visible lag on every pan/zoom
// nudge, confirmed live as the single biggest source of felt lag
// adjusting the physical rig.
//
// This page's own controls are deliberately minimal - "Exit display
// mode" and "Enable audio" below, nothing that controls anything
// remotely: picking a character, Flip, Broadcast, Trigger touch, the
// pan/zoom nudges all belong to ModelViewer.tsx, the interactive/
// controller side, precisely so this page stays a passive slate with
// nothing else to accidentally tap. Enable audio isn't an exception to
// that - it can't be, by design (see useModelCanvas's own
// attemptAudioUnlock doc comment: only a real local tap on this exact
// device can ever unlock its own audio playback, no remote action can
// do it for it) - it's a required local housekeeping affordance, not a
// remote control. Both fade the same way as ModelViewer's own controls
// (see useAutoHideControls) - a persistent button sitting in the middle
// of a physical Pepper's Ghost reflection would be a constant reminder
// this is a screen, undermining the whole illusion the rig exists for.
//
// Deliberately still tap-reactive (useModelCanvas's own tap-to-motion+
// audio isn't disabled here) - someone reaching directly into the rig
// to tap the character themselves is a harmless bonus, not something
// worth extra code to prevent.
const ModelDisplay = () => {
  const navigate = useNavigate();
  const stageRef = useRef<HTMLDivElement>(null);
  const [charCode, setCharCode] = useState<string | null>(null);
  const [flip, setFlip] = useState<string>('');
  // A controller's arrow/zoom nudge buttons (see ModelViewer.tsx) - a
  // screen-space pan/zoom correction independent of character/flip, see
  // api.ModelDisplayState.OffsetX/OffsetY/Zoom's own doc comment for why
  // it's never part of the char_code/flip payload. offsetX/offsetY
  // default to 0 (no correction) and zoom to 1 (no correction) - the
  // zero value the shared state actually reports on a server that's
  // never had SetModelDisplayTransform called is 0 for Zoom too, which
  // would render nothing at all if applied literally, so that's treated
  // as "1" here rather than trusted as a real scale factor.
  const [offsetX, setOffsetX] = useState(0);
  const [offsetY, setOffsetY] = useState(0);
  const [zoom, setZoom] = useState(1);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const { loading, error, audioUnlocked, enableAudio } = useModelCanvas(
    charCode ?? undefined,
    canvasRef,
    {
      verticalAlign: 'bottom',
      bottomMarginPx: BOTTOM_MARGIN_PX,
    },
  );
  // Not auto-hidden while loading/errored, same reasoning as
  // ModelViewer's own controls - someone setting this display up needs a
  // reliably-reachable way out if a character fails to load, not one
  // that fades away while they're stuck reading the error.
  const controlsVisible = useAutoHideControls(stageRef, loading || !!error);
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
    let socket: WebSocket | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout> | undefined;

    const applyState = (state: ModelDisplayState) => {
      setCharCode(state.char_code || null);
      setFlip(state.flip ?? '');
      setOffsetX(state.offset_x ?? 0);
      setOffsetY(state.offset_y ?? 0);
      setZoom(state.zoom || 1);

      // canvas.click() - a real DOM method, not a synthetic input event
      // - fires the exact same 'click' listener useModelCanvas's own
      // tap-to-motion+audio reaction is already attached to, so a
      // remote "Trigger touch" behaves identically to someone actually
      // touching this screen.
      if (lastTriggerRef.current === null) {
        lastTriggerRef.current = state.trigger;
      } else if (state.trigger !== lastTriggerRef.current) {
        lastTriggerRef.current = state.trigger;
        canvasRef.current?.click();
      }
    };

    // wss:// once this is ever served over https - same origin/host as
    // the page itself either way, there's no separate API host to point
    // at (see CLAUDE.md's "ships as one binary" architecture).
    const wsURL = `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/api/models/display/ws`;

    const connect = () => {
      if (cancelled) {
        return;
      }
      socket = new WebSocket(wsURL);

      socket.onmessage = (event) => {
        // The server always sends a well-formed ModelDisplayState - no
        // envelope/message-type wrapper needed, this connection only
        // ever carries one kind of message.
        applyState(JSON.parse(event.data as string) as ModelDisplayState);
      };

      // Both error and close funnel into the same reconnect - error is
      // always followed by close for a WebSocket (per spec), so there's
      // nothing extra to do here beyond letting the connection actually
      // finish closing before reconnecting.
      socket.onclose = () => {
        if (cancelled) {
          return;
        }
        reconnectTimer = setTimeout(connect, RECONNECT_DELAY_MS);
      };
    };

    connect();

    return () => {
      cancelled = true;
      clearTimeout(reconnectTimer);
      socket?.close();
    };
  }, []);

  // translate() first (leftmost/outermost) so a pan is always that many
  // real screen pixels regardless of the zoom scale or flip applied
  // after it - scale and any of FLIP_TRANSFORMS commute with each other
  // (both are diagonal/uniform, order between them never matters), but
  // translate does not commute with scale, and only the outer-applied
  // reading gives "nudge right" a fixed on-screen distance instead of
  // one that shrinks/grows with the current zoom level.
  const transformParts = [
    offsetX || offsetY ? `translate(${offsetX}px, ${offsetY}px)` : null,
    zoom !== 1 ? `scale(${zoom})` : null,
    flip ? FLIP_TRANSFORMS[flip] : null,
  ].filter(Boolean);
  const canvasTransform = transformParts.length
    ? transformParts.join(' ')
    : undefined;

  return (
    <div className={styles.stage} ref={stageRef}>
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
        style={canvasTransform ? { transform: canvasTransform } : undefined}
      />
      {/* Exit display mode - see this component's own doc comment for why
          display mode deliberately has nothing else to tap that would
          control anything remotely. "Enable audio" alongside it is a
          different category entirely, not a remote-control action - a
          real local tap on *this* device is the one thing that can ever
          unlock its own audio (see useModelCanvas's own attemptAudioUnlock
          doc comment), so this is a required local housekeeping
          affordance, and one worth being able to see rather than an
          invisible background listener with no feedback either way. */}
      <div
        className={[
          styles.controls,
          controlsVisible ? '' : styles.controlsHidden,
        ]
          .filter(Boolean)
          .join(' ')}
      >
        <button
          type="button"
          onClick={() => navigate('/models')}
          className={styles.exitButton}
          aria-label="Exit display mode and return to the character picker"
        >
          Exit display mode
        </button>
        <button
          type="button"
          onClick={enableAudio}
          className={
            audioUnlocked
              ? `${styles.exitButton} ${styles.audioEnabled}`
              : styles.exitButton
          }
          aria-label={
            audioUnlocked
              ? 'Audio is enabled for remote-triggered playback'
              : 'Tap to enable audio for remote-triggered playback'
          }
        >
          {audioUnlocked ? 'Audio: On' : 'Enable audio'}
        </button>
      </div>
    </div>
  );
};

export default ModelDisplay;

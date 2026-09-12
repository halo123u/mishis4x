import { useCallback, useEffect, useRef, useState } from 'react';
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { useModelCanvas } from '../useModelCanvas';
import { useSupportsHover } from '../useSupportsHover';
import { useAutoHideControls } from '../useAutoHideControls';
import { ModelDisplayStatus } from '../types';
import styles from './ModelViewer.module.css';

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
const BROADCAST_MODE_STORAGE_KEY = 'modelViewerBroadcastMode';

// How long "Sent!"/an error stays on the Trigger touch button before it
// reverts to its normal label - long enough to register as
// confirmation, short enough that it's obviously not a persistent mode.
const TRIGGER_STATUS_RESET_MS = 1500;

// How often to check GET /api/models/display/status while this page is
// open, to drive Trigger touch/the pan-zoom nudges' enabled state -
// separate from (and much coarser than) ModelDisplay.tsx's own real-time
// WebSocket connection, which is what actually carries every state
// change instantly. This is just "is a display currently connected at
// all," a much less latency-sensitive question - a few seconds of
// staleness on that is unnoticeable, unlike waiting on a poll to see a
// pan/zoom nudge actually land.
const DISPLAY_STATUS_POLL_INTERVAL_MS = 3000;

// How far one arrow-button click nudges the display's pan, in real
// screen pixels on the display's own screen (see ModelDisplay.tsx's
// translate(), which applies this literally) - big enough to see the
// effect on the physical rig in one tap, small enough that dialing in a
// precise position doesn't take forever.
const OFFSET_STEP_PX = 20;

// How far one zoom +/- click moves Zoom, and the range it's clamped to.
// The lower bound stops short of 0 (which would render nothing) with
// room to spare; the upper bound is an arbitrary "any more than this and
// the character is unrecognizably cropped on a small phone screen"
// judgment call, not derived from anything - easy to revisit once this
// has actually been tried against the real rig.
const ZOOM_STEP = 0.1;
const ZOOM_MIN = 0.5;
const ZOOM_MAX = 3;

// How long a pan/zoom nudge waits with no further clicks/key-repeats
// before actually pushing to the display - see schedulePushTransform's
// own doc comment for the out-of-order-network-response bug this exists
// to avoid. Comfortably longer than the gap between events in a
// deliberate rapid-tap burst or a held-down arrow key's repeat rate,
// comfortably shorter than feeling like a delay for one isolated nudge -
// and now that ModelDisplay.tsx holds a real-time WebSocket connection
// instead of polling, this debounce is the only latency left in the
// whole path from "press the key" to "see it move."
const TRANSFORM_PUSH_DEBOUNCE_MS = 150;

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
  // While the display is horizontally mirrored (flipMode 'x'), left/
  // right nudges invert - confirmed against the real rig that "right"
  // meaning screen-right (translate() is applied outside the flip
  // transform precisely so a pan is always a fixed number of real screen
  // pixels regardless of flip - see ModelDisplay.tsx's own doc comment)
  // felt backwards once you're actually looking at the mirrored result:
  // pushing "right" visibly moved the character toward what reads as its
  // own left in the reflection. Scoped to 'x' only, not '180' or 'y' -
  // '180' rotates rather than mirrors (so nothing about left/right
  // "flips" on its own, both axes invert together), and 'y' only mirrors
  // vertically, which doesn't touch left/right at all.
  const horizontalNudgeDirection = flipMode === 'x' ? -1 : 1;
  // Turns this page into a remote control for a second, separate device
  // (see ModelDisplay.tsx) - a phone mounted inside a physical rig with
  // no practical way to interact with it directly. While on, every
  // character/flip this browser lands on is also pushed to the shared
  // display state (see the broadcast effect below). Off by default so
  // ordinary browsing (or a second person just looking at a character)
  // never disturbs a live display by accident. Same dual-source
  // persistence pattern as flipMode: ?broadcast=1 for the initial setup
  // workflow, localStorage for the on-screen toggle to stick across
  // characters without needing the URL's help again.
  //
  // This replaced a one-shot "Set as display" button for a while - that
  // meant re-clicking it after every single change instead of just
  // dialing things in live, which turned out to be worse in practice
  // than the "forget it's on" risk Broadcast was originally replaced
  // over. Back to Broadcast.
  const [broadcastMode, setBroadcastMode] = useState<boolean>(() => {
    const fromQuery = searchParams.get('broadcast');
    if (fromQuery !== null) {
      return fromQuery === '1' || fromQuery === 'true';
    }
    return localStorage.getItem(BROADCAST_MODE_STORAGE_KEY) === '1';
  });
  // Same shape/reasoning as broadcastMode's transient feedback would be,
  // for the separate "Trigger touch" button below - see triggerTouch's
  // own doc comment.
  const [triggerStatus, setTriggerStatus] = useState<
    'idle' | 'sending' | 'sent' | 'error'
  >('idle');
  // Whether a display is currently believed to be polling (see
  // be/handlers/model_display.go's ModelDisplay.Connected) - gates
  // Trigger touch/the pan/zoom nudges below so they're not offering to
  // act on a display that's already gone, and (combined with
  // broadcastMode, see the `broadcasting` derived value below) whether
  // tapping the character redirects to the display instead of playing
  // locally.
  const [displayConnected, setDisplayConnected] = useState(false);
  // Broadcasting to nothing is a no-op worth treating differently from
  // broadcasting to something: with broadcastMode on but no display
  // actually connected, suppressing this controller's own local tap
  // reaction/flip rendering (see below) would make tapping the character
  // look completely dead - no local feedback and nowhere for the remote
  // one to go either. Gating on both together means that only kicks in
  // once there's an actual display to redirect to.
  const broadcasting = broadcastMode && displayConnected;
  const canvasRef = useRef<HTMLCanvasElement>(null);
  // Gates the auto-hide-after-a-stretch-of-no-interaction behavior below
  // (see useAutoHideControls's own doc comment for why it exists at all
  // on mobile) - see useSupportsHover's own doc comment for what this
  // actually detects.
  const supportsHover = useSupportsHover();
  // The display's pan/zoom correction (see ModelDisplay.tsx's own
  // offsetX/offsetY/zoom) - purely local to this controller, same as
  // flipMode's relationship to the display's own Flip: not fetched back
  // from the server on mount, so this always starts at "no correction"
  // (0, 0, 1) regardless of whatever the display was last nudged to.
  // Unlike flipMode, not persisted to localStorage/the URL either - this
  // is a one-session, dial-it-in-live control tied to correcting the
  // physical rig's current framing, not a setting worth remembering
  // across visits the way flip's acrylic-mirroring correction is.
  //
  // Only zoom is reactive state - it's the only one of the three actually
  // read during render (the zoom +/- buttons' own disabled-at-the-limit
  // props below). offsetX/offsetY live only in transformRef, since
  // nothing in this component's own render ever needs to display them.
  const [zoom, setZoom] = useState(1);
  // The live, authoritative current pan/zoom - nudgeOffset/nudgeZoom
  // read and update this synchronously rather than closing over zoom (or
  // a would-be offsetX/offsetY state pair) directly. Confirmed live this
  // was a real bug, not just theoretical: several nudge clicks fired in
  // quick succession (a physical button is exactly the kind of control
  // someone taps rapidly) all closed over the *same* not-yet-re-rendered
  // state value, so only the last click's write "won" and the rest were
  // silently lost instead of accumulating. A plain ref sidesteps React's
  // render/batching timing entirely - it's always the true current value
  // regardless of how fast clicks land.
  const transformRef = useRef({ offsetX: 0, offsetY: 0, zoom: 1 });

  // Steps null -> 'x' -> 'y' -> '180' -> null - persisted to
  // localStorage (survives navigating to a different character on this
  // same device/browser) and mirrored into the URL (survives a refresh,
  // and keeps a manually-edited ?flip= URL and the button from fighting
  // each other over which one's "right"). Doesn't push to the display
  // itself - that's the broadcast effect below's job, whenever
  // broadcastMode is on. A dedicated live-push here (from a brief
  // one-shot-"Set as display" era of this feature) got removed along
  // with it: with Broadcast back, having a *second*, independent
  // "push flip if a display happens to be connected" path regardless of
  // broadcastMode would defeat the whole point of Broadcast being the
  // one on/off switch for "is this browser currently allowed to change
  // what the display shows."
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

  // Debounces the actual network push for nudgeOffset/nudgeZoom below -
  // confirmed live this was needed, not just theoretical caution: even
  // after fixing transformRef's own stale-value bug (so every click
  // computes the right accumulated numbers), firing one PUT per click
  // still lost nudges under rapid clicking, because several independent
  // in-flight requests aren't guaranteed to *arrive* at the server in
  // the same order they were sent - a burst of clicks could have an
  // earlier click's request land last and stomp a later click's already-
  // larger value. Debouncing collapses a whole burst into a single
  // request, sent once things go quiet, carrying transformRef's true
  // final value - there's never more than one in flight, so there's
  // nothing left to race.
  const pushTransformTimeoutRef = useRef<ReturnType<typeof setTimeout>>(null);
  // useCallback with an empty dependency array - this (and nudgeOffset/
  // nudgeZoom below) only ever touch refs/constants, never anything
  // reactive, so a stable identity is both correct and what lets the
  // keydown effect further down list them as dependencies without that
  // effect re-subscribing on every render.
  const schedulePushTransform = useCallback(() => {
    if (pushTransformTimeoutRef.current) {
      clearTimeout(pushTransformTimeoutRef.current);
    }
    pushTransformTimeoutRef.current = setTimeout(() => {
      const { offsetX, offsetY, zoom } = transformRef.current;
      fetch('/api/models/display/transform', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          offset_x: offsetX,
          offset_y: offsetY,
          zoom,
        }),
      }).catch(() => {
        // Same tolerance as everything else here - a transient failure
        // just means the display keeps showing last-known framing until
        // the next successful nudge.
      });
    }, TRANSFORM_PUSH_DEBOUNCE_MS);
  }, []);

  // Arrow buttons - only meaningful while a display is connected (see
  // their own disabled prop below), so unlike flipMode this never needs
  // a "no display, just preview locally" fallback: there's no local
  // rendering of pan/zoom on this controller's own canvas to preview
  // against, the whole point is correcting where the character sits on
  // the *other* screen. Reads/writes transformRef, not React state -
  // see its own doc comment for the stale-closure bug that caused, and
  // for why offsetX/offsetY don't need to be state at all (nothing here
  // ever renders them).
  const nudgeOffset = useCallback(
    (dx: number, dy: number) => {
      transformRef.current = {
        offsetX: transformRef.current.offsetX + dx,
        offsetY: transformRef.current.offsetY + dy,
        zoom: transformRef.current.zoom,
      };
      schedulePushTransform();
    },
    [schedulePushTransform],
  );

  const nudgeZoom = useCallback(
    (delta: number) => {
      const nextZoom = Math.min(
        ZOOM_MAX,
        Math.max(
          ZOOM_MIN,
          Math.round((transformRef.current.zoom + delta) * 100) / 100,
        ),
      );
      transformRef.current = { ...transformRef.current, zoom: nextZoom };
      setZoom(nextZoom);
      schedulePushTransform();
    },
    [schedulePushTransform],
  );

  // Arrow keys/+-/= as keyboard shortcuts for the exact same nudges the
  // buttons below do - only worth having now that ModelDisplay.tsx holds
  // a real-time WebSocket connection instead of polling: dialing in
  // position by holding a key (the browser's own native key-repeat
  // fires nudgeOffset/nudgeZoom over and over, same as mashing the
  // button) only feels good once each press is visible on the physical
  // rig immediately, not up to a poll interval later. Both '+'/'=' and
  // '-'/'_' are handled for zoom - '=' and '-' are the unshifted keys on
  // a standard US layout, so this works whether or not the browser
  // reports the shifted glyph.
  //
  // Gated on displayConnected, not broadcasting - same reasoning as the
  // buttons' own disabled prop below: pan/zoom is about correcting
  // whatever's currently showing, independent of whether this browser
  // happens to be the one actively broadcasting a character right now.
  // window-level, not scoped to the stage element - there's nothing else
  // on this page that would ever want focus instead (no text inputs),
  // so there's no reason to require clicking into a specific element
  // first for the keys to register. Left/right also flow through
  // horizontalNudgeDirection - see its own doc comment.
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (!displayConnected) {
        return;
      }
      switch (event.key) {
        case 'ArrowLeft':
          event.preventDefault();
          nudgeOffset(-OFFSET_STEP_PX * horizontalNudgeDirection, 0);
          break;
        case 'ArrowRight':
          event.preventDefault();
          nudgeOffset(OFFSET_STEP_PX * horizontalNudgeDirection, 0);
          break;
        case 'ArrowUp':
          event.preventDefault();
          nudgeOffset(0, -OFFSET_STEP_PX);
          break;
        case 'ArrowDown':
          event.preventDefault();
          nudgeOffset(0, OFFSET_STEP_PX);
          break;
        case '+':
        case '=':
          event.preventDefault();
          nudgeZoom(ZOOM_STEP);
          break;
        case '-':
        case '_':
          event.preventDefault();
          nudgeZoom(-ZOOM_STEP);
          break;
        default:
          break;
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [displayConnected, nudgeOffset, nudgeZoom, horizontalNudgeDirection]);

  // Pushes this browser's current character/flip to the shared display
  // state (see ModelDisplay.tsx, which now holds this open over a real-
  // time WebSocket rather than polling it) whenever either changes, but
  // only while broadcastMode is on - see its own doc
  // comment above for why that's opt-in. Deliberately doesn't also clear
  // the display when broadcastMode turns off: turning this browser's
  // remote off shouldn't blank whatever's still live, same as unplugging
  // a TV remote doesn't turn the TV off.
  useEffect(() => {
    if (!broadcastMode || !charCode) {
      return;
    }
    fetch('/api/models/display', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        char_code: charCode,
        flip: flipMode ?? '',
      }),
    }).catch(() => {
      // Same tolerance as everything else here - a transient failure
      // just means the display keeps showing whatever it last got,
      // until the next change (or the next successful retry of this
      // same one) gets through.
    });
  }, [broadcastMode, charCode, flipMode]);

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

  // localReaction: false while actually broadcasting (see `broadcasting`
  // above) - tapping the character on this page is then remote control,
  // not a local preview, so the reaction (motion+audio) should happen on
  // whichever screen the audience is actually watching, not silently
  // also/instead fire here where nobody but the operator can see or hear
  // it. onTap fires triggerTouch every time regardless of localReaction's
  // value, so the same tap that would have played locally now drives the
  // display's own reaction instead. Gated on `broadcasting`, not plain
  // displayConnected: with a display connected but Broadcast off, this
  // browser isn't the one driving that display right now (someone else
  // might be), so a tap here should just behave like ordinary local
  // browsing. See useModelCanvas's own onTap/localReaction doc comment.
  const { loading, error } = useModelCanvas(charCode, canvasRef, {
    onTap: () => {
      if (broadcasting) {
        triggerTouch();
      }
    },
    localReaction: !broadcasting,
  });

  // Auto-hides Back/Flip/Broadcast/etc. after a stretch of no
  // interaction, and brings them back on any tap/click anywhere in the
  // stage (including one that also triggers the character's own tap-to-
  // react motion+audio - there's no need to distinguish "tapped the
  // character" from "tapped to bring controls back", both are true at
  // once). Deliberately not auto-hidden while loading or errored -
  // someone stuck on a slow load or a failure needs an obvious,
  // persistent way out, not one that fades away while they're waiting or
  // reading the error. See useAutoHideControls's own doc comment for the
  // desktop (supportsHover) half of this skip condition.
  const controlsVisible = useAutoHideControls(
    stageRef,
    loading || !!error || supportsHover,
  );

  // Reverts the Trigger touch button's transient "Sent!"/"Error" label
  // back to normal after a beat - a separate effect rather than a
  // setTimeout inside triggerTouch itself, so a rapid second click
  // cleanly restarts this timer instead of an earlier one firing
  // mid-flight and undoing the newer status.
  useEffect(() => {
    if (triggerStatus === 'idle' || triggerStatus === 'sending') {
      return;
    }
    const timer = setTimeout(
      () => setTriggerStatus('idle'),
      TRIGGER_STATUS_RESET_MS,
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
            page normally. Doesn't push anywhere by itself - see
            cycleFlipMode's own doc comment - it's the broadcast effect
            (driven by the Broadcast toggle below) that actually sends
            character/flip to the display. */}
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
        {/* See broadcastMode's own doc comment above - turns this
            browser into a remote control for a second device (see
            ModelDisplay.tsx) instead of affecting anything about this
            page's own rendering. */}
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
        {/* See triggerTouch's own doc comment - fires the display's
            tap-to-motion+audio reaction remotely. Gated on displayConnected
            alone, not broadcastMode/broadcasting: this is a one-shot poke
            at whatever's currently showing, useful regardless of whether
            this browser happens to be the one currently broadcasting a
            character to it. */}
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
        {/* Pan/zoom nudges - see nudgeOffset/nudgeZoom's own doc comment.
            Disabled whenever there's no display to correct, same
            connectivity gate as Trigger touch above and for the same
            reason: unlike Flip, there's no local rendering of this on
            the controller's own canvas to preview against, so a click
            here with nothing connected would visibly do nothing at all
            rather than just not being useful yet. ← and → always mean
            "left"/"right" as seen in the actual mirrored result, not raw
            screen-right - see horizontalNudgeDirection's own doc
            comment for why that's the same signed delta the button
            sends flips along with flipMode. */}
        <button
          type="button"
          onClick={() =>
            nudgeOffset(-OFFSET_STEP_PX * horizontalNudgeDirection, 0)
          }
          disabled={!displayConnected}
          className={styles.flipToggle}
          aria-label="Nudge the remote display's character left"
        >
          ←
        </button>
        <button
          type="button"
          onClick={() =>
            nudgeOffset(OFFSET_STEP_PX * horizontalNudgeDirection, 0)
          }
          disabled={!displayConnected}
          className={styles.flipToggle}
          aria-label="Nudge the remote display's character right"
        >
          →
        </button>
        <button
          type="button"
          onClick={() => nudgeOffset(0, -OFFSET_STEP_PX)}
          disabled={!displayConnected}
          className={styles.flipToggle}
          aria-label="Nudge the remote display's character up"
        >
          ↑
        </button>
        <button
          type="button"
          onClick={() => nudgeOffset(0, OFFSET_STEP_PX)}
          disabled={!displayConnected}
          className={styles.flipToggle}
          aria-label="Nudge the remote display's character down"
        >
          ↓
        </button>
        <button
          type="button"
          onClick={() => nudgeZoom(-ZOOM_STEP)}
          disabled={!displayConnected || zoom <= ZOOM_MIN}
          className={styles.flipToggle}
          aria-label="Zoom the remote display's character out"
        >
          −
        </button>
        <button
          type="button"
          onClick={() => nudgeZoom(ZOOM_STEP)}
          disabled={!displayConnected || zoom >= ZOOM_MAX}
          className={styles.flipToggle}
          aria-label="Zoom the remote display's character in"
        >
          +
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
        // Suppressed while actually broadcasting (see `broadcasting`
        // above): flipMode here exists to correct how the character
        // looks to whoever's watching the physical acrylic reflection,
        // and once that's the phone's job instead of this browser's,
        // this canvas rendering flipped too would just be wrong for an
        // ordinary controller screen - see "external controls should do
        // nothing on the browser when there's a display" in this
        // feature's design notes. Gated on `broadcasting`, not plain
        // displayConnected, for the same reason as localReaction above:
        // with Broadcast off, this browser isn't driving the display
        // right now, so its own canvas should render like ordinary local
        // browsing regardless of whether some display happens to be
        // connected. The toggle button above still cycles/sends flipMode
        // regardless; only its effect on *this* canvas is held back.
        style={
          flipTransform && !broadcasting
            ? { transform: flipTransform }
            : undefined
        }
      />
    </div>
  );
};

export default ModelViewer;

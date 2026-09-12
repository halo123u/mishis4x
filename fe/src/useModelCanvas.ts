import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type RefObject,
} from 'react';
import {
  AnimationState,
  AnimationStateData,
  AssetManagerBase,
  AtlasAttachmentLoader,
  Downloader,
  GLTexture,
  ManagedWebGLRenderingContext,
  ResizeMode,
  SceneRenderer,
  Skeleton,
  SkeletonBinary,
  TextureAtlas,
  Vector2,
} from '@esotericsoftware/spine-webgl';

// Matches be/cmd/model_import.go's audioClipsPerLanguage - the source
// viewer's own hardcoded voice-line rotation size, confirmed directly
// against the real host (clip 4 404s). Not fetched from the API: there's
// no listing endpoint for it (see GetCharacterModelAudioClip's own doc
// comment for why), so this is the same fixed convention the backend
// already downloads by.
const AUDIO_CLIPS_PER_LANGUAGE = 3;

// This source PNG has straight (non-premultiplied) alpha, but spine-webgl's
// Multiply blend mode (used by a couple of this character's shadow/overlay
// slots) hardcodes a blend formula that only produces correct results
// against a premultiplied source - there's no straight-alpha variant of it
// to opt into (see WebGLBlendModeConverter.getSourceColorGLBlendMode:
// Multiply always resolves to DST_COLOR regardless of the premultipliedAlpha
// flag passed to drawSkeleton). Feeding it straight-alpha pixels blows out
// any translucent-edged multiply decal toward white - visible as a
// "flashlight" glow anywhere a soft shadow/highlight overlay meets skin.
// Premultiplying the texture ourselves once at load time (synchronously,
// via a scratch 2D canvas - the image is already decoded by the time this
// runs) satisfies both blend modes at once, so drawSkeleton can use a
// single, honestly-true premultipliedAlpha flag throughout instead of the
// straight-alpha workaround from an earlier version of this component.
function loadPremultipliedTexture(
  context: ManagedWebGLRenderingContext,
  image: HTMLImageElement | ImageBitmap,
): GLTexture {
  const canvas = document.createElement('canvas');
  canvas.width = image.width;
  canvas.height = image.height;
  const ctx = canvas.getContext('2d');
  if (!ctx) {
    // No 2D context available - fall back to the source image as-is
    // rather than failing the whole load; Normal-blend slots (most of
    // the skeleton) still render correctly, only Multiply-blend ones
    // would show the glow this function exists to avoid.
    return new GLTexture(context, image);
  }
  ctx.drawImage(image, 0, 0);
  const imageData = ctx.getImageData(0, 0, canvas.width, canvas.height);
  const data = imageData.data;
  for (let i = 0; i < data.length; i += 4) {
    const a = data[i + 3] / 255;
    data[i] *= a;
    data[i + 1] *= a;
    data[i + 2] *= a;
  }
  ctx.putImageData(imageData, 0, 0);
  // GLTexture's declared type only accepts HTMLImageElement | ImageBitmap,
  // but its own upload path is just gl.texImage2D(..., this._image) - a
  // canvas is a valid TexImageSource at the real WebGL API level even
  // though these bindings don't say so.
  return new GLTexture(context, canvas as unknown as HTMLImageElement);
}

// This Spine version's AtlasAttachmentLoader throws when an attachment's
// named region is missing from the atlas, aborting the entire skeleton
// load - real imported character data legitimately hits this (a mesh
// belonging to a skin/variant the imported atlas doesn't cover). Later
// Spine versions added an `allowMissingRegions` constructor flag for
// exactly this; 4.1.11 doesn't have it, so this subclass gets the same
// effect by hand: SkeletonBinary's readAttachment (see
// node_modules/@esotericsoftware/spine-core/dist/SkeletonBinary.js)
// already treats a falsy return from these two methods as "skip this
// attachment" rather than a fatal error - only the base class's own
// unconditional throw stood in the way of that already-tolerant path.
// Param/return types derived from the base class itself, rather than
// importing Skin/Sequence/RegionAttachment/MeshAttachment directly -
// spine-webgl's package entry point doesn't re-export those low-level
// attachment/skin types even though AtlasAttachmentLoader's own .d.ts
// references them.
class TolerantAttachmentLoader extends AtlasAttachmentLoader {
  newRegionAttachment(
    ...args: Parameters<AtlasAttachmentLoader['newRegionAttachment']>
  ): ReturnType<AtlasAttachmentLoader['newRegionAttachment']> {
    try {
      return super.newRegionAttachment(...args);
    } catch (err) {
      console.warn(`missing atlas region for "${args[1]}", skipping:`, err);
      return null as unknown as ReturnType<
        AtlasAttachmentLoader['newRegionAttachment']
      >;
    }
  }

  newMeshAttachment(
    ...args: Parameters<AtlasAttachmentLoader['newMeshAttachment']>
  ): ReturnType<AtlasAttachmentLoader['newMeshAttachment']> {
    try {
      return super.newMeshAttachment(...args);
    } catch (err) {
      console.warn(`missing atlas region for "${args[1]}", skipping:`, err);
      return null as unknown as ReturnType<
        AtlasAttachmentLoader['newMeshAttachment']
      >;
    }
  }
}

// Module-level, not per-hook-call: unlocking browser autoplay policy for
// remote-triggered audio (see ensureAudioUnlockListener below) is a
// property of the browser tab/document, not of any one character or
// mount. Confirmed live this was a real bug, not just a design nicety:
// useModelCanvas's main effect already tears down and rebuilds on every
// charCode change (key={charCode} on the <canvas> forces this - a fresh
// WebGL context per character, see that key's own doc comment on both
// components) - an audioElement/unlock listener created fresh *inside*
// that effect got thrown away and recreated brand new on every single
// character switch, discarding whatever real-gesture unlock had already
// happened and silently requiring a fresh tap after every change (a
// controller broadcasting a new character to ModelDisplay.tsx, or simply
// navigating between characters, both count). A single shared element/
// unlock flag, created once and reused for the whole tab's lifetime
// regardless of how many times a character switches underneath it,
// fixes this: once it's genuinely been unlocked, it stays unlocked until
// the tab itself closes or reloads.
let sharedAudioElement: HTMLAudioElement | null = null;
function getSharedAudioElement(): HTMLAudioElement {
  if (!sharedAudioElement) {
    sharedAudioElement = new Audio();
  }
  return sharedAudioElement;
}

// sharedAudioUnlocked/audioUnlockListeners back useModelCanvas's own
// audioUnlocked/enableAudio return values - a tiny module-level pub/sub
// (not React context/state) for the same reason sharedAudioElement above
// is module-level: an "Enable audio" indicator on ModelDisplay.tsx needs
// to reflect real unlock status across every character-switch remount,
// not just the mount that happened to be live when it last changed.
// Every currently-mounted useModelCanvas call registers its own
// setAudioUnlocked here on mount and unregisters on unmount; whichever
// one actually achieves the unlock (the automatic first-tap-anywhere
// listener, or someone explicitly pressing "Enable audio") notifies all
// of them at once via markAudioUnlocked.
let sharedAudioUnlocked = false;
const audioUnlockListeners = new Set<(unlocked: boolean) => void>();

function markAudioUnlocked() {
  if (sharedAudioUnlocked) {
    return;
  }
  sharedAudioUnlocked = true;
  audioUnlockListeners.forEach((listener) => listener(true));
}

// attemptAudioUnlock is the one real implementation behind both the
// automatic pointerdown-anywhere listener and the manual "Enable audio"
// button - playing (then immediately pausing) sampleSrc on the shared
// element is what actually spends a real user gesture unlocking it, for
// remote-triggered plays as much as this specific clip.
//
// NotAllowedError specifically means the browser's autoplay policy
// itself blocked this attempt - genuinely still locked. Any other
// rejection (no audio imported for this character, a decode error, a
// 404, ...) means the policy check itself passed and playback merely
// failed for an unrelated reason - which still proves the unlock
// succeeded, so a character with no imported audio doesn't leave
// "Enable audio" stuck reporting failure forever.
function attemptAudioUnlock(sampleSrc: string) {
  const el = getSharedAudioElement();
  el.src = sampleSrc;
  void el.play().then(
    () => {
      el.pause();
      markAudioUnlocked();
    },
    (err: unknown) => {
      if (err instanceof DOMException && err.name === 'NotAllowedError') {
        return;
      }
      markAudioUnlocked();
    },
  );
}

// ensureAudioUnlockListener registers, at most once ever for this tab, a
// one-shot listener for the first real interaction anywhere on the page
// - not scoped to any specific mount's cleanup, since the whole point is
// surviving remounts. sampleSrc only matters the first time this is ever
// called (a real, playable URL is what turns the unlock attempt into a
// genuine play() the browser can actually grant, rather than an
// immediate no-source rejection of uncertain effect on the real unlock
// state) - every later mount passing a different character's URL here
// is a no-op, since audioUnlockRequested is already true by then.
let audioUnlockRequested = false;
function ensureAudioUnlockListener(sampleSrc: string) {
  if (audioUnlockRequested) {
    return;
  }
  audioUnlockRequested = true;
  document.addEventListener(
    'pointerdown',
    () => attemptAudioUnlock(sampleSrc),
    { once: true },
  );
}

// Loads and renders one imported character model (see
// be/cmd/model_import.go) into canvasRef's <canvas>, using the official
// Spine WebGL runtime, pinned to the exact Spine version (4.1.11) the
// source .skel files were exported with (confirmed by reading the
// version string embedded in a real downloaded .skel file) - a
// mismatched runtime silently reads the binary format wrong rather than
// refusing it outright. Both @esotericsoftware/spine-webgl AND
// @esotericsoftware/spine-core are pinned as exact direct dependencies
// in package.json, not just the former: Spine treats major.MINOR as the
// real compatibility boundary (a 4.1-exported .skel isn't readable by a
// 4.3 runtime), but npm's semver caret only locks the leftmost nonzero
// digit - spine-webgl's own package.json depends on spine-core via
// "^4.1.11", which npm is happy to satisfy with an already-installed,
// incompatible 4.3.x instead unless this app's own package.json pins
// spine-core down to the exact same version too.
//
// Deliberately doesn't use spine-webgl's SpineCanvas convenience wrapper:
// SpineCanvas starts a requestAnimationFrame loop with no way to stop it
// from outside, which would leak a live WebGL context every time this
// unmounts (routing away, or React 18 StrictMode's dev-only
// double-mount). Driving AssetManager/SceneRenderer directly instead
// keeps the render loop cancelable on unmount.
//
// Extracted out of ModelViewer.tsx (the original, interactive, single
// caller) so ModelDisplay.tsx - a second, passive caller with no
// buttons of its own, driven by polling instead of a URL param - can
// render the exact same way without copying ~200 lines of WebGL/Spine
// setup.
export type UseModelCanvasOptions = {
  // Called on every real tap on the canvas, regardless of
  // localReaction below - lets a caller (ModelViewer) redirect a tap
  // somewhere else (a remote trigger, see ModelDisplay.tsx) instead of,
  // or alongside, the local reaction.
  onTap?: () => void;
  // Whether a tap actually plays the local motion+audio reaction -
  // defaults to true, the original single-device behavior (see
  // ModelDisplay.tsx, which never overrides this: the display always
  // wants its own reaction, whether from a remote trigger's
  // canvas.click() or someone physically touching it). ModelViewer sets
  // this false while a remote display is connected, so tapping the
  // character on the controller redirects to the display via onTap
  // above instead of also reacting right there on the controller.
  localReaction?: boolean;
  // Anchors the character's feet a fixed margin above the bottom of the
  // canvas instead of vertically centering it (the default) - for a
  // physical rig where the mounting/acrylic angle leaves less real
  // headroom below the character than centering assumes, requiring an
  // uncomfortable posture to compensate otherwise. 'center' (or
  // omitting this) keeps the original behavior. Only ModelDisplay.tsx
  // sets this - ModelViewer's own local preview has no physical rig
  // constraint to correct for.
  verticalAlign?: 'center' | 'bottom';
  // CSS-pixel margin between the character's feet and the canvas's
  // bottom edge when verticalAlign is 'bottom' - a real CSS pixel
  // count, not a raw Spine world unit (those don't correspond to
  // screen pixels 1:1; the conversion is computed fresh every frame
  // from the renderer's actual current scale, so this stays correct
  // through a canvas resize). Ignored when verticalAlign is 'center'.
  bottomMarginPx?: number;
};

export function useModelCanvas(
  charCode: string | undefined,
  canvasRef: RefObject<HTMLCanvasElement | null>,
  options?: UseModelCanvasOptions,
): {
  loading: boolean;
  error: string | null;
  // Whether remote-triggered (or any programmatic) audio playback is
  // currently unlocked for this tab - see attemptAudioUnlock's own doc
  // comment. Only ModelDisplay.tsx's "Enable audio" button actually
  // reads this; ModelViewer ignores it, since its own audio only ever
  // plays from a real direct tap, which never needed unlocking in the
  // first place.
  audioUnlocked: boolean;
  // Manually attempts the same unlock the automatic first-tap-anywhere
  // listener does, using the current character's own clip as the sample
  // - what "Enable audio" calls on click, so someone setting up the
  // physical rig has an explicit, visible affordance instead of an
  // invisible background listener with no feedback either way.
  enableAudio: () => void;
} {
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [audioUnlocked, setAudioUnlocked] = useState(() => sharedAudioUnlocked);
  // Registers this mount's own setAudioUnlocked with the shared pub/sub
  // (see audioUnlockListeners's own doc comment) so it re-renders
  // whenever ANY mount - this one, a previous one before a character
  // switch remounted everything, or a future one - actually achieves the
  // unlock. Deliberately its own effect with an empty dependency array,
  // separate from the main per-charCode effect below: this registration
  // must survive every character switch, not tear down and reattach
  // alongside it.
  useEffect(() => {
    audioUnlockListeners.add(setAudioUnlocked);
    return () => {
      audioUnlockListeners.delete(setAudioUnlocked);
    };
  }, []);
  const enableAudio = useCallback(() => {
    if (charCode) {
      attemptAudioUnlock(`/api/models/${charCode}/audio/JP/1`);
    }
  }, [charCode]);
  // Read from inside the tap handler via .current, not closed over
  // directly - most callers pass a fresh inline options object every
  // render, and the main effect below must not re-run (tearing down
  // and rebuilding the whole WebGL context) just because e.g.
  // displayConnected flipped elsewhere in the calling component. This
  // separate, cheap effect is the only thing that needs to notice
  // options actually changing.
  const optionsRef = useRef(options);
  useEffect(() => {
    optionsRef.current = options;
  });

  useEffect(() => {
    if (!charCode || !canvasRef.current) {
      return;
    }

    setLoading(true);
    setError(null);

    const canvas = canvasRef.current;
    let disposed = false;
    let rafHandle = 0;
    // Assigned once the skeleton's actually loaded (see load() below) -
    // declared out here, not inside load(), so the cleanup function below
    // can still reach it to remove the listener regardless of how far
    // load() got before this effect unmounts.
    let onTap: (() => void) | null = null;
    // Shared across every mount/character switch, not created fresh here
    // - see sharedAudioElement's own doc comment above for the real bug
    // this fixes (a fresh, never-unlocked Audio() per character switch
    // meant losing the unlock on every single change). Chrome/Safari
    // only allow <audio>.play() to succeed *outside* a real user gesture
    // (like ModelDisplay.tsx's remote-trigger WebSocket push calling
    // canvas.click() from inside its onmessage handler) once this exact
    // element has already been played from *inside* one first -
    // confirmed live against a real Safari display that a physical tap
    // plays audio fine, but a remote-triggered one never did before this
    // element was reused rather than constructed fresh per play.
    const audioElement = getSharedAudioElement();
    ensureAudioUnlockListener(`/api/models/${charCode}/audio/JP/1`);

    const context = new ManagedWebGLRenderingContext(canvas, { alpha: true });
    const renderer = new SceneRenderer(canvas, context);
    // Not spine-webgl's own AssetManager subclass: it hardcodes its
    // textureLoader to a plain `new GLTexture(context, image)`, and
    // loadPremultipliedTexture (above) needs to be that callback instead
    // so every texture - not just this one page - gets premultiplied on
    // the way in. AssetManagerBase (the base class that subclass just
    // wraps) takes the loader directly as its first constructor argument.
    const assetManager = new AssetManagerBase(
      (image) => loadPremultipliedTexture(context, image),
      '',
      new Downloader(),
    );

    const skeletonPath = `/api/models/${charCode}/skeleton`;
    const atlasPath = `/api/models/${charCode}/atlas`;
    const texturePath = `/api/models/${charCode}/texture`;

    // This Spine version's AssetManagerBase only exposes callback-style
    // loaders (no loadBinaryAsync/loadTextureAtlasAsync yet - those came
    // later), so both are wrapped in a Promise by hand here.
    const loadBinary = (path: string) =>
      new Promise<Uint8Array>((resolve, reject) => {
        assetManager.loadBinary(
          path,
          (_path, binary) => resolve(binary),
          (_path, message) => reject(new Error(message)),
        );
      });

    // fileAlias remaps the atlas's own page image reference
    // ("char{charCode}.png", read from inside the .atlas text itself -
    // same filename be/cmd/model_import.go stores it under) to this app's
    // /texture endpoint instead of a literal filename fetch, which our
    // API doesn't serve at.
    const loadTextureAtlas = () =>
      new Promise<TextureAtlas>((resolve, reject) => {
        assetManager.loadTextureAtlas(
          atlasPath,
          (_path, atlas) => resolve(atlas),
          (_path, message) => reject(new Error(message)),
          { [`char${charCode}.png`]: texturePath },
        );
      });

    async function load() {
      const [skeletonBinary, atlas] = await Promise.all([
        loadBinary(skeletonPath),
        loadTextureAtlas(),
      ]);
      if (disposed) {
        return;
      }

      const skeletonData = new SkeletonBinary(
        new TolerantAttachmentLoader(atlas),
      ).readSkeletonData(skeletonBinary);
      const skeleton = new Skeleton(skeletonData);
      skeleton.setToSetupPose();
      skeleton.updateWorldTransform();

      const animationState = new AnimationState(
        new AnimationStateData(skeletonData),
      );
      const animationName = skeletonData.animations.some(
        (a) => a.name === 'idle',
      )
        ? 'idle'
        : skeletonData.animations[0]?.name;
      if (animationName) {
        animationState.setAnimation(0, animationName, true);
      }

      // Tap-to-react: the source game's own character screen plays a
      // one-shot "motion" clip on tap/click before settling back on
      // idle - see the reference viewer's animation dropdown, which
      // lists "motion" alongside "idle" for every character checked so
      // far. Not every imported skeleton is guaranteed to have one
      // (same "log and skip" tolerance as a missing atlas region
      // above), so this is a plain existence check rather than an
      // assumption - tapping a character without a "motion" clip is a
      // harmless no-op instead of a console error.
      const hasMotionClip = skeletonData.animations.some(
        (a) => a.name === 'motion',
      );
      // Guards against a rapid double-tap restarting "motion" mid-playback
      // (which would cut the queued return-to-idle below short and never
      // fire) - cleared by the "complete" listener once motion actually
      // finishes, not on a timer, so it tracks the real animation length
      // regardless of clip duration.
      let motionPlaying = false;
      animationState.addListener({
        complete: (entry) => {
          if (entry.animation?.name === 'motion') {
            motionPlaying = false;
          }
        },
      });

      // Voice line, cycled the same 1/2/3 round-robin the reference
      // viewer's own playNextCharacterAudio does - and, like it, only
      // triggered alongside a real "motion" play (its playCharacterMotion
      // gates playNextCharacterAudio the same way), not independently. No
      // "does this character have audio" probe beforehand: there's no
      // listing endpoint (see GetCharacterModelAudioClip's own doc
      // comment for why), so this is the same "just try it, tolerate a
      // 404" approach as hasMotionClip's own tolerance for a missing
      // clip - a character with no imported audio just plays nothing.
      // JP only, by product decision (see model-import's own
      // audioLanguages doc comment) - the API route is already
      // language-generic (GET .../audio/{language}/{clipIndex}), so
      // adding a language toggle later is a frontend-only change, not a
      // backend one.
      let nextAudioClipIndex = 1;
      const playNextAudioClip = () => {
        // Reuses audioElement (see its own doc comment above) rather
        // than constructing a new Audio() here - pause+reassign .src
        // first, same as swapping a <video>'s source, so a rapid second
        // tap cleanly interrupts whatever's still playing instead of two
        // elements overlapping.
        audioElement.pause();
        audioElement.src = `/api/models/${charCode}/audio/JP/${nextAudioClipIndex}`;
        // A missing clip (character has none, or fewer than 3) rejects
        // this promise - caught and ignored rather than logged, same
        // tolerance as everything else optional about this feature.
        void audioElement.play().catch(() => {});
        nextAudioClipIndex =
          (nextAudioClipIndex % AUDIO_CLIPS_PER_LANGUAGE) + 1;
      };

      onTap = () => {
        optionsRef.current?.onTap?.();
        if (optionsRef.current?.localReaction === false) {
          return;
        }
        if (!hasMotionClip || motionPlaying) {
          return;
        }
        motionPlaying = true;
        animationState.setAnimation(0, 'motion', false);
        if (animationName) {
          animationState.addAnimation(0, animationName, true, 0);
        }
        playNextAudioClip();
      };
      canvas.addEventListener('click', onTap);

      // No second "_face0" track: these skeletons list those alongside
      // "idle"/"motion" (see the reference viewer's own animation
      // dropdown), and it's tempting to play one simultaneously for a
      // "real" expression - but they carry baked keyframes for body
      // bones too (a common Spine export artifact: an animation clip
      // duplicated from the base rig, then only its face bones actually
      // hand-edited), so applying one on any track stamps its body pose
      // over "idle"'s own - confirmed by comparing against the reference
      // viewer, which stays seated for "idle" the whole time, unlike
      // this component briefly did with a "_face0" track added. Not
      // needed anyway: the setup pose already carries a fully-detailed
      // default face (see TolerantAttachmentLoader's doc comment for the
      // one real gap in it) - what looked like a missing expression was
      // actually the premultiplied-alpha bug below.

      // Center the camera on the skeleton's own setup-pose bounds, with a
      // little breathing room - this is a fixed character-sized viewport,
      // not a fixed pixel one, so it looks right regardless of the
      // window's actual size.
      const offset = new Vector2();
      const size = new Vector2();
      skeleton.getBounds(offset, size);
      renderer.camera.position.set(
        offset.x + size.x / 2,
        offset.y + size.y / 2,
        0,
      );
      renderer.camera.viewportWidth = size.x * 1.2;
      renderer.camera.viewportHeight = size.y * 1.2;

      setLoading(false);

      let lastTime = performance.now();
      const loop = (now: number) => {
        if (disposed) {
          return;
        }
        const delta = (now - lastTime) / 1000;
        lastTime = now;

        animationState.update(delta);
        animationState.apply(skeleton);
        skeleton.updateWorldTransform();

        renderer.resize(ResizeMode.Fit);

        // verticalAlign 'bottom' - see this option's own doc comment.
        // ResizeMode.Fit above just recomputed camera.viewportWidth/
        // viewportHeight (and already called camera.update() once) to
        // the actual effective scale for the canvas's current pixel
        // size - viewportHeight divided by the canvas's *CSS* pixel
        // height (not canvas.height, which is devicePixelRatio-scaled)
        // is exactly how many Spine world units correspond to one real
        // CSS pixel on screen right now, however big or small the
        // canvas actually is. That's what turns a plain "15" into an
        // actual, resize-safe 15 CSS pixels rather than 15 of whatever
        // arbitrary unit the source art happens to use.
        if (optionsRef.current?.verticalAlign === 'bottom') {
          const worldUnitsPerCSSPixel =
            renderer.camera.viewportHeight / canvas.clientHeight;
          const marginWorld =
            (optionsRef.current.bottomMarginPx ?? 0) * worldUnitsPerCSSPixel;
          renderer.camera.position.y =
            offset.y - marginWorld + renderer.camera.viewportHeight / 2;
          renderer.camera.update();
        }

        context.gl.clearColor(0, 0, 0, 0);
        context.gl.clear(context.gl.COLOR_BUFFER_BIT);
        renderer.begin();
        // true: textures are premultiplied by loadPremultipliedTexture
        // above, so this now matches reality throughout - see that
        // function's doc comment for why straight alpha (this component's
        // very first fix here) had to be abandoned rather than kept: it
        // fixed Normal-blend slots (the face) but broke Multiply-blend
        // ones (shadow/highlight decals), which have no straight-alpha
        // path in this runtime at all.
        renderer.drawSkeleton(skeleton, true);
        renderer.end();

        rafHandle = requestAnimationFrame(loop);
      };
      rafHandle = requestAnimationFrame(loop);
    }

    load().catch((err: unknown) => {
      if (disposed) {
        return;
      }
      // A 403 (wrong account, see ModelViewerUserID) or 404 (never
      // imported) both surface through here as a rejected message string
      // from spine-core's Downloader - not distinguishable by status code
      // at this layer, but "couldn't load" is the accurate message either
      // way.
      console.error(`error loading character model ${charCode}:`, err);
      setError('Could not load this model.');
      setLoading(false);
    });

    return () => {
      disposed = true;
      if (onTap) {
        canvas.removeEventListener('click', onTap);
      }
      // No unlock listener to remove here - it's module-level (see
      // ensureAudioUnlockListener), deliberately not tied to this
      // mount's own lifetime, so there's nothing local left to clean up.
      // audioElement itself is the shared element too - pausing it on
      // unmount still matters (don't let audio keep playing after
      // navigating away or switching characters), it just isn't this
      // mount's own object to dispose of, only to stop.
      audioElement.pause();
      if (rafHandle) {
        cancelAnimationFrame(rafHandle);
      }
      assetManager.dispose();
      renderer.dispose();
      // Explicitly releases the GPU memory behind this context right
      // now, rather than waiting on the canvas element itself to get
      // garbage collected (not a fixed timeline, especially on mobile).
      // This is the one thing actually under this component's control
      // for the "leaving the model reloads the whole page" complaint on
      // phones: the texture this canvas holds is large enough that
      // mobile Safari/Chrome will otherwise treat this as a heavy page
      // and evict it from the back-cache the moment memory gets tight,
      // turning a normal "go back" into a full network reload instead of
      // an instant restore. Freeing the GPU allocation as soon as this
      // route unmounts - not once GC eventually gets around to it - is
      // the most this component can do to make that less likely; it
      // can't force the browser to keep the previous page cached.
      context.gl.getExtension('WEBGL_lose_context')?.loseContext();
    };
  }, [charCode, canvasRef]);

  return { loading, error, audioUnlocked, enableAudio };
}

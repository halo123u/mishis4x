import { useEffect, useRef, useState } from 'react';
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';
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
import styles from './ModelViewer.module.css';

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

// Full-screen live preview of one imported character model (see
// be/cmd/model_import.go) using the official Spine WebGL runtime, pinned
// to the exact Spine version (4.1.11) the source .skel files were
// exported with (confirmed by reading the version string embedded in a
// real downloaded .skel file) - a mismatched runtime silently reads the
// binary format wrong rather than refusing it outright. Both
// @esotericsoftware/spine-webgl AND @esotericsoftware/spine-core are
// pinned as exact direct dependencies in package.json, not just the
// former: Spine treats major.MINOR as the real compatibility boundary (a
// 4.1-exported .skel isn't readable by a 4.3 runtime), but npm's semver
// caret only locks the leftmost nonzero digit - spine-webgl's own
// package.json depends on spine-core via "^4.1.11", which npm is happy
// to satisfy with an already-installed, incompatible 4.3.x instead
// unless this app's own package.json pins spine-core down to the exact
// same version too.
//
// Deliberately doesn't use spine-webgl's SpineCanvas convenience wrapper:
// SpineCanvas starts a requestAnimationFrame loop with no way to stop it
// from outside, which would leak a live WebGL context every time this
// route unmounts (routing away, or React 18 StrictMode's dev-only
// double-mount). Driving AssetManager/SceneRenderer directly instead
// keeps the render loop cancelable on unmount.
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
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
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

  useEffect(() => {
    if (!charCode || !canvasRef.current) {
      return;
    }

    const canvas = canvasRef.current;
    let disposed = false;
    let rafHandle = 0;
    // Assigned once the skeleton's actually loaded (see load() below) -
    // declared out here, not inside load(), so the cleanup function below
    // can still reach it to remove the listener regardless of how far
    // load() got before this effect unmounts.
    let onTap: (() => void) | null = null;
    // Same reasoning as onTap just above - declared out here so cleanup
    // can pause whatever voice clip is mid-playback on unmount, instead
    // of letting it keep playing after navigating away from the model.
    let currentAudio: HTMLAudioElement | null = null;

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

      // Tap-to-react: the source game's own character screen plays a one-shot "motion"
      // clip on tap/click before settling back on idle - see the reference
      // viewer's animation dropdown, which lists "motion" alongside "idle"
      // for every character checked so far. Not every imported skeleton is
      // guaranteed to have one (same "log and skip" tolerance as a missing
      // atlas region above), so this is a plain existence check rather than
      // an assumption - tapping a character without a "motion" clip is a
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
        currentAudio?.pause();
        const audio = new Audio(
          `/api/models/${charCode}/audio/JP/${nextAudioClipIndex}`,
        );
        currentAudio = audio;
        // A missing clip (character has none, or fewer than 3) rejects
        // this promise - caught and ignored rather than logged, same
        // tolerance as everything else optional about this feature.
        void audio.play().catch(() => {});
        nextAudioClipIndex =
          (nextAudioClipIndex % AUDIO_CLIPS_PER_LANGUAGE) + 1;
      };

      onTap = () => {
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
      currentAudio?.pause();
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
  }, [charCode]);

  // Auto-hides the back button after a stretch of no interaction, and
  // brings it back on any tap/click anywhere in the stage (including one
  // that also triggers the character's own tap-to-react motion+audio -
  // there's no need to distinguish "tapped the character" from "tapped
  // to bring controls back", both are true at once). A separate effect
  // from the Spine setup above: this is pure UI chrome state, unrelated
  // to loading/rendering the model itself, and touching it doesn't need
  // to re-run any of that expensive setup.
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

import { useEffect, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
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
// load - real BD2 character data legitimately hits this (a mesh
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
const ModelViewer = () => {
  const { charCode } = useParams<{ charCode: string }>();
  const navigate = useNavigate();
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

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

      // Tap-to-react: BD2's own character screen plays a one-shot "motion"
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

      // No second "_face0" track: BD2 skeletons list those alongside
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

  return (
    <div className={styles.stage}>
      {/* Real browser back, not a fixed /models link: this route is
          reachable both from the model picker (/models) and directly
          from a linked card in the collection tracker (CardModelLink,
          SetDetail.tsx) - going back should return to wherever the user
          actually came from, not always the picker. */}
      <button
        type="button"
        onClick={() => navigate(-1)}
        className={styles.back}
      >
        ← Back
      </button>
      {loading && !error && <p className={styles.status}>Loading…</p>}
      {error && <p className={styles.status}>{error}</p>}
      <canvas ref={canvasRef} className={styles.canvas} />
    </div>
  );
};

export default ModelViewer;

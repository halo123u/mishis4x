import { useEffect, useState } from 'react';
import styles from './CardThumbnail.module.css';

// Must match .preview's CSS - used here purely to keep the preview
// on-screen, never to size it (that's CSS's job).
const PREVIEW_WIDTH = 260;
const PREVIEW_HEIGHT = (PREVIEW_WIDTH * 88) / 63;
const GAP = 12;

// (hover: hover) is the standard way to ask "does this input mechanism
// support hovering at all" rather than guessing from viewport width, which
// conflates screen size with input type (a touch laptop at desktop width
// still has no real hover, and resizing a desktop window never changes
// this - a narrow window still has a real mouse).
const HOVER_QUERY = '(hover: hover) and (pointer: fine)';

// Reactive rather than a one-time check at module load - a hybrid 2-in-1
// device (or a mouse plugged into/unplugged from an otherwise touch-only
// one) can change hover capability mid-session, and which interaction mode
// applies (floating hover-preview vs. tap-to-fullscreen) should follow
// that without needing a page reload. matchMedia's own change event is
// what actually fires when this specific capability flips - deliberately
// not a resize listener, which would fire for reasons unrelated to hover/
// pointer capability entirely.
const useSupportsHover = () => {
  const [supportsHover, setSupportsHover] = useState(
    () =>
      typeof window !== 'undefined' && window.matchMedia(HOVER_QUERY).matches,
  );

  useEffect(() => {
    const mql = window.matchMedia(HOVER_QUERY);
    const onChange = (event: MediaQueryListEvent) =>
      setSupportsHover(event.matches);
    mql.addEventListener('change', onChange);
    return () => mql.removeEventListener('change', onChange);
  }, []);

  return supportsHover;
};

type CardThumbnailProps = {
  cardId: string;
  // Renders a solid gray layer over just the small thumbnail - e.g. for a
  // "missing" row - deliberately NOT implemented as a wrapping opacity:
  // opacity on an ancestor composites its entire subtree as one
  // semi-transparent layer, which would make the floating preview see-
  // through too (position: fixed escapes overflow clipping, but not an
  // ancestor's opacity). A real overlay layer only covers the thumbnail
  // itself, so the preview stays fully opaque regardless.
  dimmed?: boolean;
};

// A small card-shaped thumbnail that shows a larger version of the same
// image on interaction - shared by every card list/table so the
// interaction (and its positioning math) exists in exactly one place, not
// copied into each screen that shows card art. Hovering floats a preview
// next to the cursor on devices with real hover; tapping opens a
// full-screen preview instead on touch devices, where there's no cursor
// position to float a small box near anyway.
const CardThumbnail = ({ cardId, dimmed = false }: CardThumbnailProps) => {
  const supportsHover = useSupportsHover();
  const [preview, setPreview] = useState<{
    top: number;
    left: number;
  } | null>(null);
  const [fullscreen, setFullscreen] = useState(false);
  // If hover capability is lost while a preview happens to be open (the
  // 2-in-1-device case), there's no more mouseleave to close it - leaving
  // it as-is would make it permanently stuck rather than just a jarring
  // but harmless mid-view change. fullscreen doesn't need the same
  // treatment - its own dismiss (tap anywhere, or the close button) never
  // depended on hover in the first place. "Adjust state during render"
  // pattern (not an effect - react-hooks/set-state-in-effect flags calling
  // setState synchronously inside one), same as the imageFailed reset
  // below.
  const [lastSupportsHover, setLastSupportsHover] = useState(supportsHover);
  if (supportsHover !== lastSupportsHover) {
    setLastSupportsHover(supportsHover);
    if (!supportsHover) {
      setPreview(null);
    }
  }
  // No image uploaded for this card yet is an ordinary, expected state
  // (coverage fills in incrementally via process-set --images-dir), not an
  // error to alarm over - imageFailed swaps the <img> for a real "coming
  // soon" placeholder box instead of just hiding a broken image and
  // leaving blank space. Reset per cardId via the "adjust state during
  // render" pattern (not an effect - react-hooks/set-state-in-effect flags
  // calling setState synchronously inside one) so a component instance
  // reused across different cards, rather than remounted, doesn't keep
  // showing a stale placeholder for a card whose image actually loads
  // fine.
  const [imageFailed, setImageFailed] = useState(false);
  const [lastCardId, setLastCardId] = useState(cardId);
  if (cardId !== lastCardId) {
    setLastCardId(cardId);
    setImageFailed(false);
  }

  // position: fixed (not absolute) deliberately, so the preview escapes
  // whatever scrollable .tableWrap it's rendered inside instead of
  // getting clipped by it (per the CSS overflow spec, setting overflow on
  // one axis makes the other axis clip too, not just scroll horizontally).
  const showPreview = (event: React.MouseEvent<HTMLImageElement>) => {
    const rect = event.currentTarget.getBoundingClientRect();

    let left = rect.right + GAP;
    if (left + PREVIEW_WIDTH > window.innerWidth) {
      left = rect.left - GAP - PREVIEW_WIDTH;
    }

    let top = rect.top;
    if (top + PREVIEW_HEIGHT > window.innerHeight) {
      top = window.innerHeight - PREVIEW_HEIGHT - GAP;
    }
    top = Math.max(GAP, top);

    setPreview({ top, left });
  };

  const hidePreview = () => setPreview(null);

  return (
    <>
      <span className={styles.wrapper}>
        {imageFailed ? (
          <span className={styles.placeholder} aria-hidden="true">
            <span className={styles.placeholderLabel}>Coming soon</span>
          </span>
        ) : (
          <img
            src={`/api/cards/${cardId}/image`}
            alt=""
            className={styles.thumbnail}
            // Sets running 100+ cards render every thumbnail at once - the
            // browser's native lazy-loading defers each image's actual
            // network request until it's near the viewport, instead of
            // firing off every row's fetch on initial page load. Not
            // applied to .previewImage/.fullscreenImage below - those only
            // ever render on interaction, so they're already as lazy as it
            // gets.
            loading="lazy"
            onError={() => setImageFailed(true)}
            onMouseEnter={supportsHover ? showPreview : undefined}
            onMouseLeave={supportsHover ? hidePreview : undefined}
            onClick={supportsHover ? undefined : () => setFullscreen(true)}
          />
        )}
        {dimmed && <span className={styles.dimOverlay} aria-hidden="true" />}
      </span>

      {preview && !imageFailed && (
        <div
          className={styles.preview}
          style={{ top: preview.top, left: preview.left }}
        >
          <img
            src={`/api/cards/${cardId}/image`}
            alt=""
            className={styles.previewImage}
          />
          {dimmed && (
            <span className={styles.previewDimOverlay} aria-hidden="true" />
          )}
        </div>
      )}

      {fullscreen && !imageFailed && (
        <div
          className={styles.fullscreenOverlay}
          role="dialog"
          aria-modal="true"
          aria-label="Card preview"
          // Tapping anywhere on the overlay dismisses it, including the
          // image itself - the whole screen is the dismiss target, the
          // standard mobile lightbox pattern, rather than requiring a
          // precise tap on a small close control.
          onClick={() => setFullscreen(false)}
        >
          <button
            type="button"
            className={styles.fullscreenClose}
            aria-label="Close preview"
            onClick={() => setFullscreen(false)}
          >
            ✕
          </button>
          <span className={styles.fullscreenImageWrap}>
            <img
              src={`/api/cards/${cardId}/image`}
              alt=""
              className={styles.fullscreenImage}
            />
            {dimmed && (
              <span
                className={styles.fullscreenDimOverlay}
                aria-hidden="true"
              />
            )}
          </span>
        </div>
      )}
    </>
  );
};

export default CardThumbnail;

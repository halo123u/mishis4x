import { useState } from 'react';
import styles from './CardThumbnail.module.css';

// Must match .preview's CSS - used here purely to keep the preview
// on-screen, never to size it (that's CSS's job).
const PREVIEW_WIDTH = 260;
const PREVIEW_HEIGHT = (PREVIEW_WIDTH * 88) / 63;
const GAP = 12;

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

// A small card-shaped thumbnail that floats a larger version of the same
// image next to the cursor on hover - shared by every card list/table so
// the hover behavior (and its positioning math) exists in exactly one
// place, not copied into each screen that shows card art.
const CardThumbnail = ({ cardId, dimmed = false }: CardThumbnailProps) => {
  const [preview, setPreview] = useState<{
    top: number;
    left: number;
  } | null>(null);
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
            onError={() => setImageFailed(true)}
            onMouseEnter={showPreview}
            onMouseLeave={hidePreview}
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
    </>
  );
};

export default CardThumbnail;

import { Card, EbayListing } from '../../types';
import { priceRange } from '../../ebayListings';
import { ebaySearchUrlForQuery } from '../../ebay';
import EbayIcon from './EbayIcon';
import styles from './EbayListingsCheck.module.css';

export type EbayCheckStatus = 'idle' | 'loading' | 'loaded' | 'error';

// The "Check prices" button + range badge + listing popover for one card's
// tile (see the eBay-listings-mockups artifact's Option A - anchored
// popover on wide viewports, a full-width bottom sheet with a scrim under
// EbayListingsCheck.module.css's max-width: 30rem override, confirmed via
// real mobile screenshots that the anchored version alone overflows the
// screen edge and covers the grid's next row on a narrow layout). SetDetail
// owns all the actual fetch/open-state - this component is purely
// presentational so that state (only one card's popover open at a time)
// stays in one place.
const EbayListingsCheck = ({
  card,
  status,
  isOpen,
  listings,
  query,
  errorMessage,
  onTrigger,
  onClose,
}: {
  card: Card;
  status: EbayCheckStatus;
  isOpen: boolean;
  listings: EbayListing[];
  // The same search string the backend used to fetch listings - present
  // whenever status is 'loaded', regardless of whether any listings came
  // back. Used for the "search eBay directly" fallback link shown when
  // the API found nothing (currently the common case against sandbox -
  // see be/ebay's doc comments), rather than leaving an empty result as
  // a dead end.
  query?: string;
  errorMessage?: string;
  onTrigger: () => void;
  onClose: () => void;
}) => {
  const range = status === 'loaded' ? priceRange(listings) : null;

  return (
    <div className={styles.wrap}>
      {status !== 'loaded' && (
        <button
          type="button"
          className={
            status === 'loading'
              ? `${styles.checkBtn} ${styles.loading}`
              : styles.checkBtn
          }
          onClick={onTrigger}
          disabled={status === 'loading'}
          aria-label={`Check eBay prices for ${card.name}`}
        >
          {status === 'loading' ? (
            <span className={styles.spinner} />
          ) : (
            <EbayIcon />
          )}
          {status === 'error' ? 'Try again' : 'Check prices'}
        </button>
      )}

      {status === 'error' && errorMessage && (
        <p className={styles.error} role="alert">
          {errorMessage}
        </p>
      )}

      {status === 'loaded' && (
        <button
          type="button"
          className={styles.rangeBadge}
          onClick={onTrigger}
          aria-expanded={isOpen}
        >
          eBay{' '}
          {range ? (
            <span className={styles.amt}>
              ${(range.minCents / 100).toFixed(0)}–$
              {(range.maxCents / 100).toFixed(0)}
            </span>
          ) : (
            <span className={styles.amtMuted}>no listings found</span>
          )}
        </button>
      )}

      {isOpen && (
        <>
          <div className={`${styles.scrim} ${styles.show}`} onClick={onClose} />
          <div className={`${styles.popover} ${styles.show}`}>
            {listings.length === 0 ? (
              <div className={styles.empty}>
                <p>No current listings found.</p>
                {query && (
                  <a
                    href={ebaySearchUrlForQuery(query)}
                    target="_blank"
                    rel="noopener noreferrer"
                    className={styles.emptyLink}
                  >
                    Search eBay directly ↗
                  </a>
                )}
              </div>
            ) : (
              <>
                {range && (
                  <div className={styles.popoverHeader}>
                    <span>
                      {listings.length}{' '}
                      {listings.length === 1 ? 'listing' : 'listings'}
                    </span>
                    <span className={styles.range}>
                      $
                      <span className={styles.amt}>
                        {(range.minCents / 100).toFixed(2)}
                      </span>{' '}
                      – ${(range.maxCents / 100).toFixed(2)}
                    </span>
                  </div>
                )}
                {listings.map((listing) => (
                  <div key={listing.item_id} className={styles.listingRow}>
                    <div className={styles.listingTitle}>{listing.title}</div>
                    <div className={styles.listingMeta}>
                      {listing.condition} · {listing.seller_username} (
                      {listing.seller_feedback_percentage}%)
                    </div>
                    <div className={styles.listingPrice}>
                      <a
                        href={listing.item_web_url}
                        target="_blank"
                        rel="noopener noreferrer"
                        className={styles.listingLink}
                      >
                        ${(listing.price_cents / 100).toFixed(2)}
                      </a>
                    </div>
                  </div>
                ))}
              </>
            )}
          </div>
        </>
      )}
    </div>
  );
};

export default EbayListingsCheck;

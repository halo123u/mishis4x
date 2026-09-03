import { Card, EbayListing } from '../../types';
import { priceRange } from '../../ebayListings';
import { ebaySearchUrlForQuery } from '../../ebay';
import EbayIcon from './EbayIcon';
import styles from './EbayListingsCheck.module.css';

export type EbayCheckStatus = 'idle' | 'loading' | 'loaded' | 'error';

// The "Check prices" button + range badge for one card's tile. Once
// loaded, the badge is a plain link out to eBay's own search results
// for this card - not an in-app listing browser with its own filtering.
// eBay's own search page (condition, sort, price filters) is a better
// tool for actually picking a listing than a small in-app popover ever
// was; this app's job is just answering "roughly what does this go
// for," then handing off to eBay for anything more specific. SetDetail
// owns the actual fetch/cache state - this component is purely
// presentational.
const EbayListingsCheck = ({
  card,
  status,
  listings,
  query,
  errorMessage,
  onTrigger,
}: {
  card: Card;
  status: EbayCheckStatus;
  listings: EbayListing[];
  // The search string the backend used to fetch listings - present
  // whenever status is 'loaded', regardless of whether any listings
  // came back. Always the target of the range badge's link out to
  // eBay's real search results; falls back to the card's own code on
  // the rare chance it's missing, rather than rendering a dead link.
  query?: string;
  errorMessage?: string;
  onTrigger: () => void;
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
        <a
          href={ebaySearchUrlForQuery(query ?? card.code)}
          target="_blank"
          rel="noopener noreferrer"
          className={styles.rangeBadge}
          aria-label={`Search eBay for ${card.name}`}
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
        </a>
      )}
    </div>
  );
};

export default EbayListingsCheck;

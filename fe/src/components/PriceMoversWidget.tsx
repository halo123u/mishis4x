import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { useGlobalData } from '../useGlobalData';
import { CardPriceMover } from '../types';
import styles from './PriceMoversWidget.module.css';

// How many rows to show - the backend already returns every owned card
// with a day-over-day move, sorted by |change_cents| descending (see
// persist.GetOwnedCardPriceMovers), so this is just "how far down that
// already-sorted list to cut off" for a Home-page widget, not a query
// parameter - showing every mover in a whole collection here would
// overwhelm a small tile that's meant to be a quick glance, not the
// full picture (SetDetail's own per-card trend chart is where that
// lives).
const MAX_MOVERS = 5;

// The Home page's "today's biggest movers" widget - see
// be/persist/price_trends.go's GetOwnedCardPriceMovers for what "daily"
// actually means here (the two most recent days TCG Republic price
// history exists for a card, not a strict exactly-24h comparison).
// Gated by the same price_trends_enabled flag as SetDetail's own
// per-card trend chart - this is the same underlying feature/data,
// shipped off by default together (see handlers.Data.PriceTrendsEnabled's
// own doc comment), not a separate toggle.
const PriceMoversWidget = () => {
  const { globalData } = useGlobalData();
  const priceTrendsEnabled = globalData?.price_trends_enabled ?? false;
  const [movers, setMovers] = useState<CardPriceMover[] | null>(null);

  useEffect(() => {
    if (!priceTrendsEnabled) {
      return;
    }
    let cancelled = false;
    fetch('/api/owned-cards/price-movers')
      .then(async (res) => {
        if (!res.ok || cancelled) {
          return;
        }
        const data: CardPriceMover[] = await res.json();
        if (!cancelled) {
          setMovers(data);
        }
      })
      .catch(() => {
        // A failed fetch just leaves movers null - renderBody's own
        // "still loading" state covers this too, same tolerance as
        // every other best-effort widget fetch in this app.
      });
    return () => {
      cancelled = true;
    };
  }, [priceTrendsEnabled]);

  // Off entirely, not just an empty state - same "frontend never even
  // shows this exists while the flag's off" treatment SetDetail's own
  // trend icon gets, rather than rendering a widget that always turns
  // up nothing.
  if (!priceTrendsEnabled) {
    return null;
  }

  const renderBody = () => {
    if (movers === null) {
      return <p className={styles.status}>Loading…</p>;
    }
    if (movers.length === 0) {
      return (
        <p className={styles.status}>
          No price movement to show yet - check back after a couple of days of
          price history.
        </p>
      );
    }
    return (
      <ul className={styles.list}>
        {movers.slice(0, MAX_MOVERS).map((mover) => (
          <li key={mover.card_id}>
            <Link
              to={`/collection/${mover.set_id}`}
              className={styles.row}
              aria-label={`${mover.name}, ${mover.change_cents >= 0 ? 'up' : 'down'} ${Math.abs(mover.change_percent).toFixed(1)} percent - view its set`}
            >
              <img
                src={`/api/cards/${mover.card_id}/image`}
                alt=""
                loading="lazy"
                className={styles.thumb}
              />
              <span className={styles.name}>{mover.name}</span>
              <span
                className={
                  mover.change_cents >= 0
                    ? `${styles.change} ${styles.changeUp}`
                    : `${styles.change} ${styles.changeDown}`
                }
              >
                {mover.change_cents >= 0 ? '+' : '−'}
                {Math.abs(mover.change_percent).toFixed(1)}%
              </span>
            </Link>
          </li>
        ))}
      </ul>
    );
  };

  return (
    <section className={styles.widget}>
      <span className={styles.eyebrow}>Insights</span>
      <h2 className={styles.title}>Today&rsquo;s Biggest Movers</h2>
      {renderBody()}
    </section>
  );
};

export default PriceMoversWidget;

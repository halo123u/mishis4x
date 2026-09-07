import { FC } from 'react';
import type { Card } from '../../types';
import { computeMarketDelta } from '../../marketDelta';
import CardThumbnail from './CardThumbnail';
import RefreshIcon from './RefreshIcon';
import styles from './CardCopyStack.module.css';

export type OwnedCopy = {
  id: string;
  price_paid_cents?: number;
};

type CardCopyBrowseStackProps = {
  card: Card;
  copies: OwnedCopy[];
  activeIndex: number;
  shuffling: boolean;
  onCycle: () => void;
};

// Read-only counterpart to CardCopyStack (the editable version OnboardCards
// uses) - same stack visual and cycle interaction, but for SetDetail's
// browse view: no price input, just each copy's own recorded price
// compared directly against the card's market price. That direct,
// one-copy-to-one-copy comparison is the actual point of this component:
// market_price_cents only ever reflects a single copy (see api.Card's doc
// comment), so it was never really comparable to a multi-copy card's
// aggregate "Total paid" - SetDetail's own tile still shows that aggregate
// (now correctly scaled to market×quantity, see marketDelta.ts's doc
// comment), but this is where you can actually drill into which specific
// copy was the good or bad buy.
const CardCopyBrowseStack: FC<CardCopyBrowseStackProps> = ({
  card,
  copies,
  activeIndex,
  shuffling,
  onCycle,
}) => {
  const quantity = copies.length;
  const active = copies[activeIndex];

  const detail = (copy: OwnedCopy | undefined, indexLabel?: string) => (
    <div className={styles.browseDetail}>
      {indexLabel && <span className={styles.copyIndex}>{indexLabel}</span>}
      {copy?.price_paid_cents != null ? (
        <span className={styles.browsePrice}>
          ${(copy.price_paid_cents / 100).toFixed(2)}
        </span>
      ) : (
        <span className={styles.browsePriceUnknown}>not recorded</span>
      )}
      {/* Deliberately no last-known-price fallback here when out of
          stock (unlike SetDetail's single-copy compare row and its
          "Missing" pill, which do show it - see api.Card's LastKnown*
          doc comment) - those two are a straight text swap ("Out of
          Stock" -> "last seen $X"), same line count either way. Here it
          would add two whole new lines (a delta + a "vs. last known
          price" caption) on top of the price this copy already shows,
          which made a priced copy's stack noticeably taller than a
          card with nothing to compare at all. */}
      {copy?.price_paid_cents != null && card.market_price_cents != null && (
        <MarketDeltaLine
          paidCents={copy.price_paid_cents}
          marketCents={card.market_price_cents}
        />
      )}
    </div>
  );

  if (quantity <= 1) {
    return (
      <div className={styles.single}>
        <CardThumbnail cardId={card.id} />
        {detail(copies[0])}
      </div>
    );
  }

  return (
    <div className={styles.wrapper}>
      <div
        className={[styles.stack, shuffling ? styles.shuffling : '']
          .filter(Boolean)
          .join(' ')}
        role="button"
        tabIndex={0}
        aria-label={`Cycle through owned copies of ${card.name} (copy ${activeIndex + 1} of ${quantity})`}
        onClick={onCycle}
        onKeyDown={(event) => {
          if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault();
            onCycle();
          }
        }}
      >
        {quantity >= 3 && (
          <span
            className={`${styles.peek} ${styles.peek2}`}
            aria-hidden="true"
          />
        )}
        <span className={`${styles.peek} ${styles.peek1}`} aria-hidden="true" />
        <span className={styles.front}>
          <CardThumbnail cardId={card.id} />
        </span>
        <button
          type="button"
          className={styles.cycleBtn}
          aria-label={`Show next copy of ${card.name}`}
          onClick={(event) => {
            event.stopPropagation();
            onCycle();
          }}
        >
          <RefreshIcon />
        </button>
      </div>

      <div className={styles.dots}>
        {copies.map((copy, i) => (
          <span
            key={copy.id}
            className={[
              styles.dot,
              copy.price_paid_cents != null ? styles.dotPriced : '',
              i === activeIndex ? styles.dotActive : '',
            ]
              .filter(Boolean)
              .join(' ')}
          />
        ))}
      </div>

      {detail(active, `Copy ${activeIndex + 1} of ${quantity}`)}
    </div>
  );
};

const deltaToneClass = {
  good: styles.deltaGood,
  bad: styles.deltaBad,
  muted: styles.deltaMuted,
};

const MarketDeltaLine: FC<{ paidCents: number; marketCents: number }> = ({
  paidCents,
  marketCents,
}) => {
  const delta = computeMarketDelta(paidCents, marketCents);
  return (
    <div className={`${styles.delta} ${deltaToneClass[delta.tone]}`}>
      {delta.label}
    </div>
  );
};

export default CardCopyBrowseStack;

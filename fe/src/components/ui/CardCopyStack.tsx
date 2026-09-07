import { FC } from 'react';
import CardThumbnail from './CardThumbnail';
import RefreshIcon from './RefreshIcon';
import styles from './CardCopyStack.module.css';

// One physical copy, as OnboardCards tracks it client-side while editing -
// id is present for a copy the server already has (round-tripped from
// GET /api/owned-sets/{setID}/cards, see types.ts's OwnedCardCopyInput),
// absent for one added this session that doesn't exist yet. price is the
// same dollar-string convention OnboardCards already used for its single
// price field (not cents, not a number - see OnboardCards' dollarsToCents
// for why), just one per copy now instead of one per card.
export type CardCopyDraft = {
  id?: string;
  price: string;
};

type CardCopyStackProps = {
  cardId: string;
  cardName: string;
  copies: CardCopyDraft[];
  activeIndex: number;
  shuffling: boolean;
  onCycle: () => void;
  onPriceChange: (value: string) => void;
  onPriceBlur: () => void;
  disabled?: boolean;
};

// Renders a card's owned copies (#108) - a single flat thumbnail + one
// price box for 0 or 1 copies (identical to the pre-#108 experience,
// deliberately no stack gimmick when there's nothing to represent), or a
// short solitaire-style stack for 2+: two faded "peek" layers behind the
// real thumbnail suggesting depth, a cycle button (or tapping the stack
// itself) advancing to the next copy, dots showing count and which copies
// have a price on file yet, and one editable price box bound to whichever
// copy is currently active. See the design-mock artifact this was built
// from for the full reasoning.
//
// The peek layers are plain chrome, not a second/third CardThumbnail -
// every copy of a card shares identical art (see CardCopyDraft's doc
// comment), so there's nothing distinct to show behind the front one;
// only the price/index readout actually changes as you cycle.
const CardCopyStack: FC<CardCopyStackProps> = ({
  cardId,
  cardName,
  copies,
  activeIndex,
  shuffling,
  onCycle,
  onPriceChange,
  onPriceBlur,
  disabled = false,
}) => {
  const quantity = copies.length;
  const active = copies[activeIndex];

  const priceBox = (copy: CardCopyDraft | undefined, label: string) => (
    <span className={styles.priceInputWrap}>
      <span className={styles.priceCurrency} aria-hidden="true">
        $
      </span>
      <input
        type="number"
        min={0}
        step="0.01"
        placeholder="0"
        aria-label={label}
        className={styles.priceInput}
        value={copy?.price ?? ''}
        onChange={(event) => onPriceChange(event.target.value)}
        onBlur={onPriceBlur}
        disabled={quantity === 0 || disabled}
      />
    </span>
  );

  if (quantity <= 1) {
    return (
      <div className={styles.single}>
        <CardThumbnail cardId={cardId} />
        {priceBox(copies[0], `Price paid for ${cardName}`)}
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
        tabIndex={disabled ? -1 : 0}
        aria-label={`Cycle through owned copies of ${cardName} (copy ${activeIndex + 1} of ${quantity})`}
        onClick={disabled ? undefined : onCycle}
        onKeyDown={
          disabled
            ? undefined
            : (event) => {
                if (event.key === 'Enter' || event.key === ' ') {
                  event.preventDefault();
                  onCycle();
                }
              }
        }
      >
        {quantity >= 3 && (
          <span
            className={`${styles.peek} ${styles.peek2}`}
            aria-hidden="true"
          />
        )}
        <span className={`${styles.peek} ${styles.peek1}`} aria-hidden="true" />
        <span className={styles.front}>
          <CardThumbnail cardId={cardId} />
        </span>
        <button
          type="button"
          className={styles.cycleBtn}
          aria-label={`Show next copy of ${cardName}`}
          disabled={disabled}
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
            key={copy.id ?? `new-${i}`}
            className={[
              styles.dot,
              copy.price.trim() !== '' ? styles.dotPriced : '',
              i === activeIndex ? styles.dotActive : '',
            ]
              .filter(Boolean)
              .join(' ')}
          />
        ))}
      </div>

      <span
        className={[styles.copyIndex, shuffling ? styles.fading : '']
          .filter(Boolean)
          .join(' ')}
      >
        Copy {activeIndex + 1} of {quantity}
      </span>

      {priceBox(
        active,
        `Price paid for copy ${activeIndex + 1} of ${cardName}`,
      )}
    </div>
  );
};

export default CardCopyStack;

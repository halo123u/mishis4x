import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import type { Card, OwnedCardInput, Set as SetT } from '../types';
import styles from './DeckInsights.module.css';

type OwnedEntry = {
  quantity: number;
  pricePaidCents?: number;
};

// A card's market price for every aggregate below: its current price when
// in stock, falling back to the last real price it ever had (see
// api.Card's LastKnownMarket* doc comment) when it's out of stock right
// now - so a card doesn't just vanish from Owned Market Value/Cost to
// Complete/Most Valuable the moment TCG Republic temporarily runs out.
// Still undefined for a card that's never had a real price recorded at
// all (checked and found nothing, every time) - there's genuinely nothing
// to use there, same as before this fallback existed.
const effectiveMarketPriceCents = (card: Card): number | undefined =>
  card.market_price_cents ?? card.last_known_market_price_cents;

// Splits the cards missing from a value's coverage into *why* they're
// missing - "out of stock" (checked, and neither a current nor a last
// known price - a real, current answer, not a gap) vs. "not tracked yet"
// (never checked at all, e.g. no card_price_sources row configured).
// Market Value/Cost to Complete both silently exclude both groups from
// their sums, which reads as "this card just isn't priced" if left
// unexplained - this is the breakdown that makes clear which of those two
// very different reasons applies, matching the same market_checked_at
// distinction marketUnavailableLabel uses per-card on SetDetail.
const coverageBreakdown = (cards: Card[]): string => {
  const outOfStock = cards.filter(
    (card) =>
      card.market_checked_at != null && effectiveMarketPriceCents(card) == null,
  ).length;
  const notTracked = cards.filter(
    (card) => card.market_checked_at == null,
  ).length;

  return [
    outOfStock > 0 ? `${outOfStock} out of stock` : null,
    notTracked > 0 ? `${notTracked} not tracked yet` : null,
  ]
    .filter((s): s is string => s !== null)
    .join(', ');
};

// How many of cards are only priced via the last-known fallback above,
// not a live current price.
const usingLastKnownCount = (cards: Card[]): number =>
  cards.filter(
    (card) =>
      card.market_price_cents == null &&
      card.last_known_market_price_cents != null,
  ).length;

// The full parenthetical coverage caption - "2 using last known price, 1
// not tracked yet" - shared by the page-level coverage note (owned cards)
// and the Cost to Complete stat's own caption (missing cards) so the
// wording can't drift between them. Empty string (render nothing, no
// stray "()") when cards has nothing to call out either way.
const coverageNote = (cards: Card[], pricedCount: number): string => {
  const lastKnown = usingLastKnownCount(cards);
  const parts = [
    lastKnown > 0 ? `${lastKnown} using last known price` : null,
    cards.length - pricedCount > 0 ? coverageBreakdown(cards) : null,
  ].filter((s): s is string => s !== null);
  return parts.length > 0 ? ` (${parts.join(', ')})` : '';
};

// Thin wrapper so navigating directly between two sets' insights pages
// fully remounts DeckInsightsContent via the key change, same pattern
// SetDetail uses for the same reason.
const DeckInsights = () => {
  const { setID } = useParams<{ setID: string }>();
  return <DeckInsightsContent key={setID} setID={setID} />;
};

const DeckInsightsContent = ({ setID }: { setID?: string }) => {
  const [cards, setCards] = useState<Card[] | null>(null);
  const [owned, setOwned] = useState<Record<string, OwnedEntry>>({});
  const [setName, setSetName] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [rarityFilter, setRarityFilter] = useState('all');

  useEffect(() => {
    if (!setID) {
      return;
    }

    Promise.all([
      fetch(`/api/sets/${setID}/cards`),
      fetch(`/api/owned-sets/${setID}/cards`),
      fetch('/api/sets'),
    ])
      .then(async ([cardsRes, ownedRes, allSetsRes]) => {
        if (cardsRes.status === 404) {
          setError('This set could not be found.');
          return;
        }
        if (cardsRes.status !== 200 || ownedRes.status !== 200) {
          setError('Could not load insights. Please try again.');
          return;
        }

        const ownedCards: OwnedCardInput[] = await ownedRes.json();
        const ownedMap: Record<string, OwnedEntry> = {};
        for (const oc of ownedCards) {
          if (oc.quantity > 0) {
            ownedMap[oc.card_id] = {
              quantity: oc.quantity,
              pricePaidCents: oc.price_paid_cents,
            };
          }
        }
        setOwned(ownedMap);
        setCards(await cardsRes.json());

        // Not fatal - the header just falls back to a generic title.
        if (allSetsRes.status === 200) {
          const allSets: SetT[] = await allSetsRes.json();
          setSetName(allSets.find((s) => s.id === setID)?.name ?? null);
        }
      })
      .catch(() => {
        setError('Could not reach the server. Please try again.');
      });
  }, [setID]);

  // Same first-appearance-order convention as SetDetail's own rarity
  // filter, for a consistent feel between the two pages.
  const rarities: string[] = [];
  for (const card of cards ?? []) {
    if (!rarities.includes(card.rarity)) {
      rarities.push(card.rarity);
    }
  }

  const filteredCards = (cards ?? []).filter(
    (card) => rarityFilter === 'all' || card.rarity === rarityFilter,
  );

  const ownedCards = filteredCards.filter(
    (card) => (owned[card.id]?.quantity ?? 0) > 0,
  );
  const missingCards = filteredCards.filter(
    (card) => (owned[card.id]?.quantity ?? 0) === 0,
  );

  const ownedWithMarket = ownedCards.filter(
    (card) => effectiveMarketPriceCents(card) != null,
  );
  const ownedMarketValueCents = ownedWithMarket.reduce(
    (sum, card) => sum + effectiveMarketPriceCents(card)!,
    0,
  );

  const totalPaidCents = ownedCards.reduce(
    (sum, card) => sum + (owned[card.id]?.pricePaidCents ?? 0),
    0,
  );

  // Only cards with *both* a known paid price and a known market price are
  // fair to compare - summing paid across all owned cards against market
  // value across only the priced subset would silently conflate two
  // different denominators.
  const comparableCards = ownedCards.filter(
    (card) =>
      effectiveMarketPriceCents(card) != null &&
      owned[card.id]?.pricePaidCents != null,
  );
  const comparablePaidCents = comparableCards.reduce(
    (sum, card) => sum + owned[card.id]!.pricePaidCents!,
    0,
  );
  const comparableMarketCents = comparableCards.reduce(
    (sum, card) => sum + effectiveMarketPriceCents(card)!,
    0,
  );
  const deltaCents = comparablePaidCents - comparableMarketCents;

  const missingWithMarket = missingCards.filter(
    (card) => effectiveMarketPriceCents(card) != null,
  );
  const costToCompleteCents = missingWithMarket.reduce(
    (sum, card) => sum + effectiveMarketPriceCents(card)!,
    0,
  );

  const topValuable = [...ownedWithMarket]
    .sort(
      (a, b) => effectiveMarketPriceCents(b)! - effectiveMarketPriceCents(a)!,
    )
    .slice(0, 5);

  return (
    <div className="stack">
      <div className={styles.header}>
        <Link to="/collection" className={styles.back}>
          ← Back to sets
        </Link>
        <h1 className={styles.title}>{setName ?? 'Deck'} — Insights</h1>
      </div>

      {error && (
        <p className={styles.error} role="alert">
          {error}
        </p>
      )}

      {!error && cards === null && <p className="muted">Loading insights…</p>}

      {cards && cards.length > 0 && (
        <div className={styles.filters}>
          <select
            aria-label="Filter by rarity"
            className={styles.raritySelect}
            value={rarityFilter}
            onChange={(event) => setRarityFilter(event.target.value)}
          >
            <option value="all">All rarities</option>
            {rarities.map((rarity) => (
              <option key={rarity} value={rarity}>
                {rarity}
              </option>
            ))}
          </select>
        </div>
      )}

      {cards && cards.length > 0 && (
        <>
          <p className={styles.coverageNote}>
            Based on {ownedWithMarket.length} of {ownedCards.length} owned cards
            with market data
            {coverageNote(ownedCards, ownedWithMarket.length)}
          </p>

          <div className={styles.statGrid}>
            <div className={styles.statCard}>
              <span className={styles.statLabel}>Owned Market Value</span>
              <span className={styles.statValue}>
                ${(ownedMarketValueCents / 100).toFixed(2)}
              </span>
              <span className={styles.statSub}>
                {ownedWithMarket.length} priced cards
              </span>
            </div>
            <div className={styles.statCard}>
              <span className={styles.statLabel}>Total Paid</span>
              <span className={styles.statValue}>
                ${(totalPaidCents / 100).toFixed(2)}
              </span>
              <span className={styles.statSub}>
                {ownedCards.length} owned cards
              </span>
            </div>
            <div className={styles.statCard}>
              <span className={styles.statLabel}>Paid vs. Market</span>
              {comparableCards.length > 0 ? (
                <>
                  <span
                    className={`${styles.statValue} ${deltaCents <= 0 ? styles.good : styles.bad}`}
                  >
                    {deltaCents === 0
                      ? 'At market'
                      : `${deltaCents < 0 ? '▼' : '▲'} $${(Math.abs(deltaCents) / 100).toFixed(2)} ${deltaCents < 0 ? 'under' : 'over'}`}
                  </span>
                  <span className={styles.statSub}>
                    across {comparableCards.length} directly comparable cards
                  </span>
                </>
              ) : (
                <>
                  <span className={`${styles.statValue} ${styles.statMuted}`}>
                    —
                  </span>
                  <span className={styles.statSub}>
                    no cards with both a paid and market price yet
                  </span>
                </>
              )}
            </div>
            <div className={styles.statCard}>
              <span className={styles.statLabel}>Cost to Complete</span>
              <span className={styles.statValue}>
                ${(costToCompleteCents / 100).toFixed(2)}
              </span>
              <span className={styles.statSub}>
                {missingWithMarket.length} of {missingCards.length} missing
                cards priced
                {coverageNote(missingCards, missingWithMarket.length)}
              </span>
            </div>
            <div className={styles.statCard}>
              <span className={styles.statLabel}>Completion</span>
              <span className={styles.statValue}>
                {ownedCards.length} / {filteredCards.length}
              </span>
              <span className={styles.statSub}>
                {filteredCards.length > 0
                  ? `${((ownedCards.length / filteredCards.length) * 100).toFixed(1)}%`
                  : '—'}
              </span>
            </div>
          </div>

          {topValuable.length > 0 && (
            <div className={styles.topCards}>
              <h2 className={styles.topCardsHeading}>
                Most Valuable Owned Cards
              </h2>
              <div className={styles.topCardList}>
                {topValuable.map((card, i) => (
                  <div key={card.id} className={styles.topCardRow}>
                    <span className={styles.rank}>{i + 1}</span>
                    <img
                      src={`/api/cards/${card.id}/image`}
                      alt=""
                      className={styles.topCardImage}
                    />
                    <span className={styles.topCardName}>
                      <span>{card.name}</span>
                      <span>
                        {card.code} · {card.rarity}
                      </span>
                    </span>
                    <span className={styles.topCardValue}>
                      ${(effectiveMarketPriceCents(card)! / 100).toFixed(2)}
                    </span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
};

export default DeckInsights;

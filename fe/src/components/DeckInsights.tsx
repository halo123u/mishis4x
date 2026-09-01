import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import type { Card, OwnedCardInput, Set as SetT } from '../types';
import styles from './DeckInsights.module.css';

type OwnedEntry = {
  quantity: number;
  pricePaidCents?: number;
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
    (card) => card.market_price_cents != null,
  );
  const ownedMarketValueCents = ownedWithMarket.reduce(
    (sum, card) => sum + card.market_price_cents!,
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
      card.market_price_cents != null && owned[card.id]?.pricePaidCents != null,
  );
  const comparablePaidCents = comparableCards.reduce(
    (sum, card) => sum + owned[card.id]!.pricePaidCents!,
    0,
  );
  const comparableMarketCents = comparableCards.reduce(
    (sum, card) => sum + card.market_price_cents!,
    0,
  );
  const deltaCents = comparablePaidCents - comparableMarketCents;

  const missingWithMarket = missingCards.filter(
    (card) => card.market_price_cents != null,
  );
  const costToCompleteCents = missingWithMarket.reduce(
    (sum, card) => sum + card.market_price_cents!,
    0,
  );

  const topValuable = [...ownedWithMarket]
    .sort((a, b) => b.market_price_cents! - a.market_price_cents!)
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
            with current market data
            {ownedCards.length - ownedWithMarket.length > 0 &&
              ` (${ownedCards.length - ownedWithMarket.length} owned cards aren't tracked yet)`}
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
                      ${(card.market_price_cents! / 100).toFixed(2)}
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

import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { Card, OwnedCardInput, Set as SetT } from '../types';
import Button from './ui/Button';
import CardThumbnail from './ui/CardThumbnail';
import EbayIcon from './ui/EbayIcon';
import { ebaySearchUrl } from '../ebay';
import styles from './SetDetail.module.css';

// market_checked_at being set (regardless of market_price_cents) means the
// sync job has actually looked at this card before and found nothing to
// report - "Out of Stock" is an honest read of that. Its absence means no
// price source has ever been checked for this card at all - a different,
// less specific state worth saying plainly rather than guessing.
const marketUnavailableLabel = (card: Card): string =>
  card.market_checked_at != null ? 'Out of Stock' : 'Not tracked yet';

// Thin wrapper so navigating directly between two sets (/collection/A ->
// /collection/B) fully remounts SetDetailContent via the key change,
// resetting its state naturally instead of needing to reset it by hand
// inside the effect (which react-hooks/set-state-in-effect flags, and
// which would otherwise flash A's stale cards before B's fetch resolves).
const SetDetail = () => {
  const { setID } = useParams<{ setID: string }>();
  return <SetDetailContent key={setID} setID={setID} />;
};

const SetDetailContent = ({ setID }: { setID?: string }) => {
  const [cards, setCards] = useState<Card[] | null>(null);
  // Only needed for the eBay quick-link's search query - GET /api/sets is
  // the full catalog list, not just this one set, but there's no
  // single-set lookup endpoint and the list itself is small/cheap.
  const [setName, setSetName] = useState<string | null>(null);
  // card_id -> quantity, only for cards with a quantity > 0 owned_cards
  // row - a card missing from this map reads as "not owned" whether that's
  // because there's no row at all or an explicit quantity-0 one, which is
  // the right distinction for this read-only view (SetDetail doesn't need
  // to tell those two apart the way the editor form does).
  const [owned, setOwned] = useState<Record<string, number> | null>(null);
  // card_id -> price_paid_cents, only for owned cards with a known price -
  // a card missing here just means "unknown," same as an unowned card
  // missing from `owned` above.
  const [ownedPrices, setOwnedPrices] = useState<Record<string, number>>({});
  const [error, setError] = useState<string | null>(null);
  // Deleting is a destructive, unrecoverable action (it clears card
  // ownership too, not just the set marker) - confirmingDelete gates a
  // second, explicit click behind an inline prompt instead of a native
  // confirm() dialog, matching the rest of the app's hand-rolled UI.
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);
  // Filtering only affects which rows render below - same as the
  // onboarding/edit screen's filter bar.
  const [search, setSearch] = useState('');
  const [rarityFilter, setRarityFilter] = useState('all');
  const [ownershipFilter, setOwnershipFilter] = useState<
    'all' | 'owned' | 'missing'
  >('all');
  const navigate = useNavigate();

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
          setError('Could not load cards. Please try again.');
          return;
        }

        const ownedCards: OwnedCardInput[] = await ownedRes.json();
        const ownedMap: Record<string, number> = {};
        const pricesMap: Record<string, number> = {};
        for (const oc of ownedCards) {
          if (oc.quantity > 0) {
            ownedMap[oc.card_id] = oc.quantity;
            if (oc.price_paid_cents != null) {
              pricesMap[oc.card_id] = oc.price_paid_cents;
            }
          }
        }
        setOwned(ownedMap);
        setOwnedPrices(pricesMap);
        setCards(await cardsRes.json());

        // Not fatal if this one fails - the eBay link just falls back to
        // omitting the set name from its query rather than blocking the
        // whole page over a non-essential extra.
        if (allSetsRes.status === 200) {
          const allSets: SetT[] = await allSetsRes.json();
          setSetName(allSets.find((s) => s.id === setID)?.name ?? null);
        }
      })
      .catch(() => {
        setError('Could not reach the server. Please try again.');
      });
  }, [setID]);

  const handleDelete = () => {
    if (!setID) {
      return;
    }

    setDeleting(true);
    setDeleteError(null);

    fetch(`/api/owned-sets/${setID}`, { method: 'DELETE' })
      .then((res) => {
        if (res.status !== 204) {
          throw new Error('delete-failed');
        }
        navigate('/collection');
      })
      .catch(() => {
        setDeleteError('Could not remove this set. Please try again.');
        setDeleting(false);
      });
  };

  // In first-appearance order rather than alphabetically - matches the
  // set's own rarity progression (1-star before 2-star before 3-star,
  // etc.), same as the onboarding/edit screen's dropdown.
  const rarities: string[] = [];
  for (const card of cards ?? []) {
    if (!rarities.includes(card.rarity)) {
      rarities.push(card.rarity);
    }
  }

  // Rarity + search only, not the owned/missing toggle - this is the
  // denominator for the summary stats below, so filtering by rarity (e.g.
  // "SP") correctly shrinks "Owned: X / Y" to that rarity's own count.
  // Deliberately excludes ownershipFilter: applying that too would make
  // "Owned" mode always read "X / X" and "Missing" mode always "0 / X" -
  // a summary that's trivially true isn't useful information.
  const normalizedSearch = search.trim().toLowerCase();
  const rarityAndSearchFilteredCards = (cards ?? []).filter((card) => {
    if (rarityFilter !== 'all' && card.rarity !== rarityFilter) {
      return false;
    }
    if (!normalizedSearch) {
      return true;
    }
    return (
      card.code.toLowerCase().includes(normalizedSearch) ||
      card.name.toLowerCase().includes(normalizedSearch)
    );
  });

  // Adds the owned/missing toggle on top, for what the table itself shows.
  const visibleCards = rarityAndSearchFilteredCards.filter((card) => {
    const isOwned = (owned?.[card.id] ?? 0) > 0;
    if (ownershipFilter === 'owned' && !isOwned) {
      return false;
    }
    if (ownershipFilter === 'missing' && isOwned) {
      return false;
    }
    return true;
  });

  // Sum of every owned card's known price, and a distinct-card completion
  // count (not a copy count - owning 3x the same card still only counts
  // once) - both scoped to rarityAndSearchFilteredCards so they track
  // whatever rarity/search filter is active, per the same logic above.
  const totalPaidCents = rarityAndSearchFilteredCards.reduce(
    (sum, card) => sum + (ownedPrices[card.id] ?? 0),
    0,
  );
  const ownedCount = rarityAndSearchFilteredCards.filter(
    (card) => (owned?.[card.id] ?? 0) > 0,
  ).length;
  const totalCount = rarityAndSearchFilteredCards.length;

  return (
    <div className="stack">
      <div className={styles.header}>
        <Link to="/collection" className={styles.back}>
          ← Back to sets
        </Link>
        {setID && (
          <div className={styles.headerActions}>
            <Link
              to={`/collection/${setID}/onboard`}
              state={{ from: 'detail' }}
            >
              <Button variant="secondary">Edit collection</Button>
            </Link>
            <Button variant="danger" onClick={() => setConfirmingDelete(true)}>
              Delete set
            </Button>
          </div>
        )}
      </div>

      {confirmingDelete && (
        <div className={styles.confirmDelete} role="alert">
          <p>
            Remove this set and everything you've tracked for it? This can't be
            undone.
          </p>
          <div className={styles.confirmActions}>
            <Button variant="danger" onClick={handleDelete} disabled={deleting}>
              {deleting ? 'Removing…' : 'Yes, delete'}
            </Button>
            <Button
              variant="secondary"
              onClick={() => setConfirmingDelete(false)}
              disabled={deleting}
            >
              Cancel
            </Button>
          </div>
        </div>
      )}

      {deleteError && (
        <p className={styles.error} role="alert">
          {deleteError}
        </p>
      )}

      {error && (
        <p className={styles.error} role="alert">
          {error}
        </p>
      )}

      {!error && cards === null && <p className="muted">Loading cards…</p>}

      {cards && cards.length === 0 && (
        <p className="muted">This set doesn't have any cards yet.</p>
      )}

      {cards && cards.length > 0 && (
        <div className={styles.summaryRow}>
          <p className={styles.summary}>
            Owned:{' '}
            <span className={styles.summaryValue}>
              {ownedCount} / {totalCount}
            </span>
          </p>
          <p className={styles.summary}>
            Total paid:{' '}
            <span className={styles.summaryValue}>
              ${(totalPaidCents / 100).toFixed(2)}
            </span>
          </p>
        </div>
      )}

      {cards && cards.length > 0 && (
        <div className={styles.filters}>
          <input
            type="search"
            placeholder="Search by code or name"
            aria-label="Search by code or name"
            className={styles.searchInput}
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
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
          <select
            aria-label="Filter by ownership"
            className={styles.raritySelect}
            value={ownershipFilter}
            onChange={(event) =>
              setOwnershipFilter(
                event.target.value as 'all' | 'owned' | 'missing',
              )
            }
          >
            <option value="all">All</option>
            <option value="owned">Owned</option>
            <option value="missing">Missing</option>
          </select>
        </div>
      )}

      {cards && cards.length > 0 && visibleCards.length === 0 && (
        <p className="muted">No cards match your filter.</p>
      )}

      {cards && cards.length > 0 && visibleCards.length > 0 && (
        <div className={styles.gridWrap}>
          <div className={styles.grid}>
            {visibleCards.map((card) => {
              const quantity = owned?.[card.id] ?? 0;
              return (
                <div
                  key={card.id}
                  className={
                    quantity === 0
                      ? `${styles.tile} ${styles.tileMissing}`
                      : styles.tile
                  }
                >
                  <CardThumbnail cardId={card.id} dimmed={quantity === 0} />
                  <div className={styles.tileName}>{card.name}</div>
                  <div className={styles.tileCode}>{card.code}</div>
                  <span className={styles.rarityChip}>{card.rarity}</span>
                  <div className={styles.tileStats}>
                    {quantity > 0 ? (
                      <span className={styles.owned}>×{quantity}</span>
                    ) : (
                      <span className={styles.missing}>Missing</span>
                    )}
                    {quantity > 0 && ownedPrices[card.id] != null && (
                      <span className={styles.price}>
                        ${(ownedPrices[card.id] / 100).toFixed(2)}
                      </span>
                    )}
                  </div>
                  {quantity > 0
                    ? // Owned: paid vs. market, when there's something to
                      // compare - a card the user hasn't priced yet, or one
                      // with no current market data, just shows what it does
                      // have rather than a misleading delta.
                      (ownedPrices[card.id] != null ||
                        card.market_price_cents != null) && (
                        <div className={styles.compareRow}>
                          {card.market_price_cents != null ? (
                            <>
                              <div className={styles.compareLine}>
                                <span>Market</span>
                                <span>
                                  ${(card.market_price_cents / 100).toFixed(2)}
                                </span>
                              </div>
                              {ownedPrices[card.id] != null &&
                                (() => {
                                  const deltaCents =
                                    ownedPrices[card.id] -
                                    card.market_price_cents!;
                                  if (deltaCents === 0) {
                                    return (
                                      <div
                                        className={`${styles.delta} ${styles.deltaMuted}`}
                                      >
                                        At market price
                                      </div>
                                    );
                                  }
                                  const under = deltaCents < 0;
                                  return (
                                    <div
                                      className={`${styles.delta} ${under ? styles.deltaGood : styles.deltaBad}`}
                                    >
                                      {under ? '▼' : '▲'} $
                                      {(Math.abs(deltaCents) / 100).toFixed(2)}{' '}
                                      {under ? 'under' : 'over'} market
                                    </div>
                                  );
                                })()}
                            </>
                          ) : (
                            <div
                              className={`${styles.delta} ${styles.deltaMuted}`}
                            >
                              {marketUnavailableLabel(card)}
                            </div>
                          )}
                        </div>
                      )
                    : // Missing: no "paid" to compare against, so just the
                      // raw market price (or why there isn't one) - nothing
                      // extra when this card has never been tracked at all,
                      // "Missing" above already says enough for that case.
                      (card.market_price_cents != null ||
                        card.market_checked_at != null) && (
                        <div
                          className={
                            card.market_price_cents != null
                              ? styles.marketPill
                              : `${styles.marketPill} ${styles.marketPillMuted}`
                          }
                        >
                          {card.market_price_cents != null
                            ? `Market $${(card.market_price_cents / 100).toFixed(2)}`
                            : marketUnavailableLabel(card)}
                        </div>
                      )}
                  <a
                    href={ebaySearchUrl(setName, card.code)}
                    target="_blank"
                    rel="noopener noreferrer"
                    className={styles.ebayLink}
                    aria-label={`Search eBay for ${card.name}`}
                  >
                    <EbayIcon />
                  </a>
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
};

export default SetDetail;

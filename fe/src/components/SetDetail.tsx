import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { Card, OwnedCardInput } from '../types';
import Button from './ui/Button';
import CardThumbnail from './ui/CardThumbnail';
import styles from './SetDetail.module.css';

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

  // Sum of every owned card's known price - ownedPrices is already scoped
  // to owned (quantity > 0) cards with a recorded price, so this is exactly
  // "what you've actually logged spending on this set," not an estimate.
  const totalPaidCents = Object.values(ownedPrices).reduce(
    (sum, cents) => sum + cents,
    0,
  );
  // Distinct cards owned (any quantity > 0), out of the set's full card
  // count - a completion count, not a copy count, so owning 3x the same
  // card still only counts once here. Deliberately computed over the full
  // set regardless of the filter bar below, same as totalPaidCents.
  const ownedCount = Object.keys(owned ?? {}).length;
  const totalCount = cards?.length ?? 0;

  useEffect(() => {
    if (!setID) {
      return;
    }

    Promise.all([
      fetch(`/api/sets/${setID}/cards`),
      fetch(`/api/owned-sets/${setID}/cards`),
    ])
      .then(async ([cardsRes, ownedRes]) => {
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

  const normalizedSearch = search.trim().toLowerCase();
  const visibleCards = (cards ?? []).filter((card) => {
    if (rarityFilter !== 'all' && card.rarity !== rarityFilter) {
      return false;
    }
    const isOwned = (owned?.[card.id] ?? 0) > 0;
    if (ownershipFilter === 'owned' && !isOwned) {
      return false;
    }
    if (ownershipFilter === 'missing' && isOwned) {
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
        <div className={styles.tableWrap}>
          <table className={styles.table}>
            <thead>
              <tr>
                <th className={styles.thumbnailCol}></th>
                <th>Code</th>
                <th>Name</th>
                <th>Rarity</th>
                <th>Owned</th>
                <th>Price paid</th>
              </tr>
            </thead>
            <tbody>
              {visibleCards.map((card) => {
                const quantity = owned?.[card.id] ?? 0;
                return (
                  <tr
                    key={card.id}
                    className={quantity === 0 ? styles.rowMissing : undefined}
                  >
                    <td className={styles.thumbnailCol}>
                      <CardThumbnail cardId={card.id} dimmed={quantity === 0} />
                    </td>
                    <td className={styles.code}>{card.code}</td>
                    <td>{card.name}</td>
                    <td>{card.rarity}</td>
                    <td>
                      {quantity > 0 ? (
                        <span className={styles.owned}>×{quantity}</span>
                      ) : (
                        <span className={styles.missing}>Missing</span>
                      )}
                    </td>
                    <td>
                      {quantity > 0 && ownedPrices[card.id] != null ? (
                        <span className={styles.price}>
                          ${(ownedPrices[card.id] / 100).toFixed(2)}
                        </span>
                      ) : (
                        <span className={styles.missing}>—</span>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
};

export default SetDetail;

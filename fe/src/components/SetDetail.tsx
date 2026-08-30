import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { Card, OwnedCardInput } from '../types';
import Button from './ui/Button';
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
  const navigate = useNavigate();

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
        <div className={styles.tableWrap}>
          <table className={styles.table}>
            <thead>
              <tr>
                <th>Code</th>
                <th>Name</th>
                <th>Rarity</th>
                <th>Owned</th>
                <th>Price paid</th>
              </tr>
            </thead>
            <tbody>
              {cards.map((card) => {
                const quantity = owned?.[card.id] ?? 0;
                return (
                  <tr
                    key={card.id}
                    className={quantity === 0 ? styles.rowMissing : undefined}
                  >
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

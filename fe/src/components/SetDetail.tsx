import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
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
  const [error, setError] = useState<string | null>(null);

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
        for (const oc of ownedCards) {
          if (oc.quantity > 0) {
            ownedMap[oc.card_id] = oc.quantity;
          }
        }
        setOwned(ownedMap);
        setCards(await cardsRes.json());
      })
      .catch(() => {
        setError('Could not reach the server. Please try again.');
      });
  }, [setID]);

  return (
    <div className="stack">
      <div className={styles.header}>
        <Link to="/collection" className={styles.back}>
          ← Back to sets
        </Link>
        {setID && (
          <Link to={`/collection/${setID}/onboard`} state={{ from: 'detail' }}>
            <Button variant="secondary">Edit collection</Button>
          </Link>
        )}
      </div>

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
        <table className={styles.table}>
          <thead>
            <tr>
              <th>Code</th>
              <th>Name</th>
              <th>Rarity</th>
              <th>Owned</th>
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
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
    </div>
  );
};

export default SetDetail;

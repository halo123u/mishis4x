import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { Card } from '../types';
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
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!setID) {
      return;
    }

    fetch(`/api/sets/${setID}/cards`)
      .then(async (res) => {
        if (res.status === 404) {
          setError('This set could not be found.');
          return;
        }
        if (res.status !== 200) {
          setError('Could not load cards. Please try again.');
          return;
        }
        setCards(await res.json());
      })
      .catch(() => {
        setError('Could not reach the server. Please try again.');
      });
  }, [setID]);

  return (
    <div className="stack">
      <Link to="/collection" className={styles.back}>
        ← Back to sets
      </Link>

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
            </tr>
          </thead>
          <tbody>
            {cards.map((card) => (
              <tr key={card.id}>
                <td className={styles.code}>{card.code}</td>
                <td>{card.name}</td>
                <td>{card.rarity}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
};

export default SetDetail;

import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { Set } from '../types';
import styles from './CollectionDashboard.module.css';

const CollectionDashboard = () => {
  const [sets, setSets] = useState<Set[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    fetch('/api/sets')
      .then(async (res) => {
        if (res.status !== 200) {
          setError('Could not load sets. Please try again.');
          return;
        }
        setSets(await res.json());
      })
      .catch(() => {
        setError('Could not reach the server. Please try again.');
      });
  }, []);

  return (
    <div className="stack">
      <h1>Card Manager</h1>

      {error && (
        <p className={styles.error} role="alert">
          {error}
        </p>
      )}

      {!error && sets === null && <p className="muted">Loading sets…</p>}

      {sets && sets.length === 0 && (
        <p className="muted">
          No sets have been added yet - check back once the catalog import job
          has run.
        </p>
      )}

      {sets && sets.length > 0 && (
        <ul className={styles.list}>
          {sets.map((set) => (
            <li key={set.id}>
              <Link to={`/collection/${set.id}`} className={styles.setCard}>
                <span className={styles.setName}>{set.name}</span>
                <span className={styles.setMeta}>
                  {set.card_count} cards · {set.status}
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
};

export default CollectionDashboard;

import { useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import type { Set as SetT } from '../types';
import Button from './ui/Button';
import styles from './CollectionDashboard.module.css';

// Shows the sets the user has actually onboarded (GET /api/owned-sets),
// not the full catalog (GET /api/sets) - a fresh account starts empty here
// even once the catalog doesn't, and "Add a set" is how that gap gets
// closed rather than the dashboard just listing everything that exists.
const CollectionDashboard = () => {
  const [sets, setSets] = useState<SetT[] | null>(null);
  // Whether the catalog has any set left that isn't already owned - the
  // "Add a set" button only makes sense to show while this is true, same
  // logic AddSet itself uses to decide its own empty state.
  const [hasMoreToAdd, setHasMoreToAdd] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const navigate = useNavigate();

  useEffect(() => {
    Promise.all([fetch('/api/owned-sets'), fetch('/api/sets')])
      .then(async ([ownedRes, allRes]) => {
        if (ownedRes.status !== 200 || allRes.status !== 200) {
          setError('Could not load your sets. Please try again.');
          return;
        }

        const owned: SetT[] = await ownedRes.json();
        const all: SetT[] = await allRes.json();
        const ownedIDs = new Set(owned.map((s) => s.id));

        setHasMoreToAdd(all.some((s) => !ownedIDs.has(s.id)));
        setSets(owned);
      })
      .catch(() => {
        setError('Could not reach the server. Please try again.');
      });
  }, []);

  return (
    <div className="stack">
      <div className={styles.header}>
        <h1>Card Manager</h1>
        {sets && hasMoreToAdd && (
          <Button onClick={() => navigate('/collection/add')}>Add a set</Button>
        )}
      </div>

      {error && (
        <p className={styles.error} role="alert">
          {error}
        </p>
      )}

      {!error && sets === null && <p className="muted">Loading your sets…</p>}

      {sets && sets.length === 0 && (
        <p className="muted">
          You haven't added any sets yet - click "Add a set" to get started.
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

import { useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import type { Set as SetT } from '../types';
import Button from './ui/Button';
import styles from './AddSet.module.css';

// The onboarding flow's "pick a set" step: GET /api/sets is the full
// catalog, GET /api/owned-sets is what's already onboarded - this shows
// the difference, so a set already added doesn't show up here again.
// Right now that's just the one seeded set (more catalog models/CSV
// imports are still to come), but the flow itself doesn't assume that.
const AddSet = () => {
  const [available, setAvailable] = useState<SetT[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [addingID, setAddingID] = useState<string | null>(null);
  const navigate = useNavigate();

  useEffect(() => {
    Promise.all([fetch('/api/sets'), fetch('/api/owned-sets')])
      .then(async ([allRes, ownedRes]) => {
        if (allRes.status !== 200 || ownedRes.status !== 200) {
          setError('Could not load sets. Please try again.');
          return;
        }

        const all: SetT[] = await allRes.json();
        const owned: SetT[] = await ownedRes.json();
        const ownedIDs = new Set(owned.map((s) => s.id));

        setAvailable(all.filter((s) => !ownedIDs.has(s.id)));
      })
      .catch(() => {
        setError('Could not reach the server. Please try again.');
      });
  }, []);

  const handleAdd = (setID: string) => {
    setAddingID(setID);
    setError(null);

    fetch('/api/owned-sets', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ set_id: setID }),
    })
      .then((res) => {
        if (res.status !== 204) {
          setError('Could not add that set. Please try again.');
          setAddingID(null);
          return;
        }
        navigate(`/collection/${setID}`);
      })
      .catch(() => {
        setError('Could not reach the server. Please try again.');
        setAddingID(null);
      });
  };

  return (
    <div className="stack">
      <Link to="/collection" className={styles.back}>
        ← Back to sets
      </Link>

      <h1>Add a set</h1>

      {error && (
        <p className={styles.error} role="alert">
          {error}
        </p>
      )}

      {!error && available === null && (
        <p className="muted">Loading available sets…</p>
      )}

      {available && available.length === 0 && (
        <p className="muted">You've already added every set in the catalog.</p>
      )}

      {available && available.length > 0 && (
        <ul className={styles.list}>
          {available.map((set) => (
            <li key={set.id} className={styles.setCard}>
              <div>
                <span className={styles.setName}>{set.name}</span>
                <span className={styles.setMeta}>
                  {set.card_count} cards · {set.status}
                </span>
              </div>
              <Button
                onClick={() => handleAdd(set.id)}
                disabled={addingID === set.id}
              >
                {addingID === set.id ? 'Adding…' : 'Add'}
              </Button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
};

export default AddSet;

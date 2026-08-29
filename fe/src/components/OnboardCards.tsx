import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import type { Card } from '../types';
import Button from './ui/Button';
import styles from './OnboardCards.module.css';

// The onboarding flow's "which cards do you actually own" step, shown right
// after AddSet's "Add" click for a given set. Submitting here does two
// things at once: onboards the set itself (POST /api/owned-sets, same call
// AddSet used to make directly) and records ownership + quantity for
// whichever cards got checked (POST /api/owned-sets/{setID}/cards) - so a
// user who never submits this form never ends up with a set marked owned
// but no card data, and one who does gets both in one step.
const OnboardCards = () => {
  const { setID } = useParams<{ setID: string }>();
  const [cards, setCards] = useState<Card[] | null>(null);
  const [selected, setSelected] = useState<Record<string, boolean>>({});
  const [quantities, setQuantities] = useState<Record<string, number>>({});
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const navigate = useNavigate();

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

  const toggleCard = (cardID: string) => {
    setSelected((prev) => ({ ...prev, [cardID]: !prev[cardID] }));
    setQuantities((prev) => (prev[cardID] ? prev : { ...prev, [cardID]: 1 }));
  };

  const setQuantity = (cardID: string, quantity: number) => {
    setQuantities((prev) => ({ ...prev, [cardID]: Math.max(1, quantity) }));
  };

  // Onboards the set (idempotent either way) and, if any cards are passed,
  // records their ownership in the same submit. "Skip for now" calls this
  // with an empty list regardless of what's checked, rather than reusing
  // whatever the checkboxes currently hold.
  const submit = (selectedCards: { card_id: string; quantity: number }[]) => {
    if (!setID) {
      return;
    }

    setSubmitting(true);
    setError(null);

    fetch('/api/owned-sets', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ set_id: setID }),
    })
      .then((res) => {
        if (res.status !== 204) {
          throw new Error('add-set-failed');
        }
        if (selectedCards.length === 0) {
          return Promise.resolve(new Response(null, { status: 204 }));
        }
        return fetch(`/api/owned-sets/${setID}/cards`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ cards: selectedCards }),
        });
      })
      .then((res) => {
        if (res.status !== 204) {
          throw new Error('set-cards-failed');
        }
        navigate(`/collection/${setID}`);
      })
      .catch(() => {
        setError('Could not save your collection. Please try again.');
        setSubmitting(false);
      });
  };

  return (
    <div className="stack">
      <Link to="/collection/add" className={styles.back}>
        ← Back to add a set
      </Link>

      <h1>Which cards do you own?</h1>
      <p className="muted">
        Check off the cards you have and how many, or skip this for now and add
        them later.
      </p>

      {error && (
        <p className={styles.error} role="alert">
          {error}
        </p>
      )}

      {!error && cards === null && <p className="muted">Loading cards…</p>}

      {cards && cards.length > 0 && (
        <table className={styles.table}>
          <thead>
            <tr>
              <th className={styles.checkboxCol}>Owned</th>
              <th>Code</th>
              <th>Name</th>
              <th>Rarity</th>
              <th className={styles.quantityCol}>Qty</th>
            </tr>
          </thead>
          <tbody>
            {cards.map((card) => (
              <tr key={card.id}>
                <td className={styles.checkboxCol}>
                  <input
                    type="checkbox"
                    aria-label={`I own ${card.name}`}
                    checked={!!selected[card.id]}
                    onChange={() => toggleCard(card.id)}
                    disabled={submitting}
                  />
                </td>
                <td className={styles.code}>{card.code}</td>
                <td>{card.name}</td>
                <td>{card.rarity}</td>
                <td className={styles.quantityCol}>
                  <input
                    type="number"
                    min={1}
                    aria-label={`Quantity of ${card.name}`}
                    className={styles.quantityInput}
                    value={quantities[card.id] ?? 1}
                    onChange={(event) =>
                      setQuantity(card.id, Number(event.target.value))
                    }
                    disabled={!selected[card.id] || submitting}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {cards && (
        <div className={styles.actions}>
          <Button
            onClick={() =>
              submit(
                Object.keys(selected)
                  .filter((cardID) => selected[cardID])
                  .map((cardID) => ({
                    card_id: cardID,
                    quantity: quantities[cardID] ?? 1,
                  })),
              )
            }
            disabled={submitting}
          >
            {submitting ? 'Saving…' : 'Save my collection'}
          </Button>
          <Button
            variant="secondary"
            onClick={() => submit([])}
            disabled={submitting}
          >
            Skip for now
          </Button>
        </div>
      )}
    </div>
  );
};

export default OnboardCards;

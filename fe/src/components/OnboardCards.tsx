import { useEffect, useState } from 'react';
import { Link, useLocation, useNavigate, useParams } from 'react-router-dom';
import type { Card, OwnedCardInput } from '../types';
import Button from './ui/Button';
import styles from './OnboardCards.module.css';

// The "which cards do you own" step - reused for two entry points, told
// apart by how we got here (see backTo below): AddSet's "Add" click for a
// brand new set, where every row starts unchecked, and SetDetail's "Edit
// collection" button for a set already onboarded, where rows pre-fill from
// GET /api/owned-sets/{setID}/cards. Either way, submitting onboards the
// set itself (POST /api/owned-sets, idempotent) alongside recording card
// ownership, so an abandoned form never leaves a set marked owned with no
// card data.
const OnboardCards = () => {
  const { setID } = useParams<{ setID: string }>();
  const location = useLocation();
  const [cards, setCards] = useState<Card[] | null>(null);
  const [selected, setSelected] = useState<Record<string, boolean>>({});
  const [quantities, setQuantities] = useState<Record<string, number>>({});
  // Dollar-amount text as typed, not cents - kept as a string (rather than
  // a number) so a card with no known price is a genuinely empty input,
  // not a "0" the user would have to delete first, and so mid-edit text
  // like "12." isn't clobbered by re-parsing on every keystroke.
  const [prices, setPrices] = useState<Record<string, string>>({});
  // The quantities we loaded on mount, for cards with quantity > 0 only -
  // used on save to tell "unchecked, was never owned" (nothing to submit)
  // apart from "unchecked, but WAS owned" (must submit an explicit 0 to
  // actually clear it - see handleSave).
  const [initiallyOwned, setInitiallyOwned] = useState<Record<string, number>>(
    {},
  );
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const navigate = useNavigate();

  // Arrived via SetDetail's "Edit collection" button - go back there
  // instead of the add-a-set picker (which wouldn't make sense mid-edit),
  // and adjust the copy below to match ("skip this" reads oddly once
  // there's already something to skip past).
  const isEditing =
    (location.state as { from?: string } | null)?.from === 'detail';
  const backTo =
    isEditing && setID ? `/collection/${setID}` : '/collection/add';

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

        const owned: OwnedCardInput[] = await ownedRes.json();
        const ownedNow: Record<string, number> = {};
        const initiallySelected: Record<string, boolean> = {};
        const initialPrices: Record<string, string> = {};
        for (const oc of owned) {
          if (oc.quantity > 0) {
            ownedNow[oc.card_id] = oc.quantity;
            initiallySelected[oc.card_id] = true;
            if (oc.price_paid_cents != null) {
              initialPrices[oc.card_id] = (oc.price_paid_cents / 100).toFixed(
                2,
              );
            }
          }
        }
        setInitiallyOwned(ownedNow);
        setQuantities(ownedNow);
        setPrices(initialPrices);
        setSelected(initiallySelected);

        setCards(await cardsRes.json());
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

  const setPrice = (cardID: string, value: string) => {
    setPrices((prev) => ({ ...prev, [cardID]: value }));
  };

  // undefined for "no price entered" (blank, or not a real number yet
  // mid-typing) - cents (rounded, to sidestep float cruft like 12.1*100)
  // for a real amount.
  const priceCentsFor = (cardID: string): number | undefined => {
    const raw = prices[cardID];
    if (!raw || raw.trim() === '') {
      return undefined;
    }
    const dollars = Number(raw);
    if (Number.isNaN(dollars)) {
      return undefined;
    }
    return Math.round(dollars * 100);
  };

  // Onboards the set (idempotent either way) and, if any cards are passed,
  // records their ownership in the same submit. "Skip for now" calls this
  // with an empty list regardless of what's checked or was previously
  // owned - it never touches card ownership, only the set itself.
  const submit = (
    selectedCards: {
      card_id: string;
      quantity: number;
      price_paid_cents?: number;
    }[],
  ) => {
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

  // Checked cards submit their quantity (and price, if entered) as usual.
  // A card that WAS owned (initiallyOwned) but is no longer checked has to
  // be submitted too, explicitly at quantity 0 - otherwise SetOwnedCards
  // never hears about it and the old ownership row just sits there
  // unchanged. A card that was never owned and stays unchecked is left out
  // entirely, same as before - no need to create a "never interacted" row
  // for it. Price is only ever sent for checked cards - an unchecked card
  // being cleared to quantity 0 has no meaningful price to keep either.
  const handleSave = () => {
    const toSubmit = cards ?? [];
    submit(
      toSubmit
        .filter((card) => selected[card.id] || initiallyOwned[card.id])
        .map((card) => ({
          card_id: card.id,
          quantity: selected[card.id] ? (quantities[card.id] ?? 1) : 0,
          price_paid_cents: selected[card.id]
            ? priceCentsFor(card.id)
            : undefined,
        })),
    );
  };

  return (
    <div className="stack">
      <Link to={backTo} className={styles.back}>
        ← Back
      </Link>

      <h1>Which cards do you own?</h1>
      <p className="muted">
        {isEditing
          ? 'Check off the cards you have and how many.'
          : 'Check off the cards you have and how many, or skip this for now and add them later.'}
      </p>

      {error && (
        <p className={styles.error} role="alert">
          {error}
        </p>
      )}

      {!error && cards === null && <p className="muted">Loading cards…</p>}

      {cards && cards.length > 0 && (
        <div className={styles.tableWrap}>
          <table className={styles.table}>
            <thead>
              <tr>
                <th className={styles.checkboxCol}>Owned</th>
                <th>Code</th>
                <th>Name</th>
                <th>Rarity</th>
                <th className={styles.quantityCol}>Qty</th>
                <th className={styles.priceCol}>Price paid</th>
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
                  <td className={styles.priceCol}>
                    <span className={styles.priceInputWrap}>
                      <span className={styles.priceCurrency} aria-hidden="true">
                        $
                      </span>
                      <input
                        type="number"
                        min={0}
                        step="0.01"
                        placeholder="Optional"
                        aria-label={`Price paid for ${card.name}`}
                        className={styles.priceInput}
                        value={prices[card.id] ?? ''}
                        onChange={(event) =>
                          setPrice(card.id, event.target.value)
                        }
                        disabled={!selected[card.id] || submitting}
                      />
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {cards && (
        <div className={styles.actions}>
          <Button onClick={handleSave} disabled={submitting}>
            {submitting ? 'Saving…' : 'Save my collection'}
          </Button>
          <Button
            variant="secondary"
            onClick={() => submit([])}
            disabled={submitting}
          >
            {isEditing ? 'Discard changes' : 'Skip for now'}
          </Button>
        </div>
      )}
    </div>
  );
};

export default OnboardCards;

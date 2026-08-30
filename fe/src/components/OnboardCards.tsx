import { useEffect, useState } from 'react';
import { Link, useLocation, useNavigate, useParams } from 'react-router-dom';
import type { Card, OwnedCardInput } from '../types';
import Button from './ui/Button';
import CardThumbnail from './ui/CardThumbnail';
import QuantityStepper from './ui/QuantityStepper';
import styles from './OnboardCards.module.css';

// The "which cards do you own" step - reused for two entry points, told
// apart by how we got here (see backTo below): AddSet's "Add" click for a
// brand new set, where every row starts at quantity 0, and SetDetail's
// "Edit collection" button for a set already onboarded, where rows
// pre-fill from GET /api/owned-sets/{setID}/cards. Either way, submitting
// onboards the set itself (POST /api/owned-sets, idempotent) alongside
// recording card ownership, so an abandoned form never leaves a set
// marked owned with no card data.
const OnboardCards = () => {
  const { setID } = useParams<{ setID: string }>();
  const location = useLocation();
  const [cards, setCards] = useState<Card[] | null>(null);
  // card_id -> quantity is the *only* ownership state now - there's no
  // separate checkbox to keep in sync with it. A card's presence in this
  // map (not its value) is what decides whether it gets submitted at all:
  // populated on load from every row the server already has (including an
  // explicit 0 one, if a previous edit left it that way), and added to
  // lazily the first time this session's stepper touches a card the
  // server never had a row for. 0 just means "not owned," submitted the
  // same way any other quantity would be - no separate "explicitly
  // cleared" case to track, because there's no longer a second piece of
  // state that could drift from it.
  const [quantities, setQuantities] = useState<Record<string, number>>({});
  // Dollar-amount text as typed, not cents - kept as a string (rather than
  // a number) so a card with no known price is a genuinely empty input,
  // not a "0" the user would have to delete first, and so mid-edit text
  // like "12." isn't clobbered by re-parsing on every keystroke.
  const [prices, setPrices] = useState<Record<string, string>>({});
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  // Filtering only affects which rows render below - quantities/prices stay
  // keyed by card.id regardless, so toggling a filter never loses input on
  // a row that's momentarily hidden.
  const [search, setSearch] = useState('');
  const [rarityFilter, setRarityFilter] = useState('all');
  const [ownershipFilter, setOwnershipFilter] = useState<
    'all' | 'owned' | 'missing'
  >('all');
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
        // Every returned row seeds the quantities map, including an
        // explicit 0 one - it still needs to be "touched" so re-saving
        // without changing it submits that same 0 again, not silently
        // drops the row from the request entirely.
        const ownedNow: Record<string, number> = {};
        const initialPrices: Record<string, string> = {};
        for (const oc of owned) {
          ownedNow[oc.card_id] = oc.quantity;
          if (oc.quantity > 0 && oc.price_paid_cents != null) {
            initialPrices[oc.card_id] = (oc.price_paid_cents / 100).toFixed(2);
          }
        }
        setQuantities(ownedNow);
        setPrices(initialPrices);

        setCards(await cardsRes.json());
      })
      .catch(() => {
        setError('Could not reach the server. Please try again.');
      });
  }, [setID]);

  // Used by both QuantityStepper's arrows and typing directly into its
  // field - either way, 0 is a real, valid value (not owned), just never
  // negative.
  const setQuantityDirect = (cardID: string, quantity: number) => {
    setQuantities((prev) => ({ ...prev, [cardID]: Math.max(0, quantity) }));
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
  // with an empty list regardless of what quantities are set or were
  // previously owned - it never touches card ownership, only the set
  // itself.
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

  // A card only gets submitted if it's actually in the quantities map -
  // one the server already had a row for, or one this session's stepper
  // touched. A card nobody has ever interacted with (never owned, never
  // clicked this session) is left out entirely, same as before - no need
  // to create a "never interacted" row for it. Price is only ever sent
  // alongside a real (>0) quantity - a card sitting at 0 has no meaningful
  // price to keep either.
  const handleSave = () => {
    const toSubmit = cards ?? [];
    submit(
      toSubmit
        .filter((card) => card.id in quantities)
        .map((card) => ({
          card_id: card.id,
          quantity: quantities[card.id],
          price_paid_cents:
            quantities[card.id] > 0 ? priceCentsFor(card.id) : undefined,
        })),
    );
  };

  // In first-appearance order rather than alphabetically - that already
  // matches the set's own rarity progression (1-star before 2-star before
  // 3-star, etc.), so the dropdown reads the same way the table is sorted.
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
    const isOwned = (quantities[card.id] ?? 0) > 0;
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
      <Link to={backTo} className={styles.back}>
        ← Back
      </Link>

      <h1>Which cards do you own?</h1>
      <p className="muted">
        {isEditing
          ? 'Set how many of each you have.'
          : 'Set how many of each you have, or skip this for now and add them later.'}
      </p>

      {error && (
        <p className={styles.error} role="alert">
          {error}
        </p>
      )}

      {!error && cards === null && <p className="muted">Loading cards…</p>}

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
            <option value="all">Owned + missing</option>
            <option value="owned">Owned only</option>
            <option value="missing">Missing only</option>
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
                <th className={styles.quantityCol}>Qty</th>
                <th className={styles.priceCol}>Price paid</th>
              </tr>
            </thead>
            <tbody>
              {visibleCards.map((card) => {
                const quantity = quantities[card.id] ?? 0;
                return (
                  <tr key={card.id}>
                    <td className={styles.thumbnailCol}>
                      <CardThumbnail cardId={card.id} />
                    </td>
                    <td className={styles.code}>{card.code}</td>
                    <td>{card.name}</td>
                    <td>{card.rarity}</td>
                    <td className={styles.quantityCol}>
                      <QuantityStepper
                        value={quantity}
                        onChange={(next) => setQuantityDirect(card.id, next)}
                        ariaLabel={`quantity of ${card.name}`}
                        disabled={submitting}
                      />
                    </td>
                    <td className={styles.priceCol}>
                      <span className={styles.priceInputWrap}>
                        <span
                          className={styles.priceCurrency}
                          aria-hidden="true"
                        >
                          $
                        </span>
                        <input
                          type="number"
                          min={0}
                          step="0.01"
                          placeholder="0"
                          aria-label={`Price paid for ${card.name}`}
                          className={styles.priceInput}
                          value={prices[card.id] ?? ''}
                          onChange={(event) =>
                            setPrice(card.id, event.target.value)
                          }
                          disabled={quantity === 0 || submitting}
                        />
                      </span>
                    </td>
                  </tr>
                );
              })}
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

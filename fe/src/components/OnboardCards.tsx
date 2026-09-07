import { useEffect, useState } from 'react';
import { Link, useLocation, useNavigate, useParams } from 'react-router-dom';
import type { Card, OwnedCardInput, Set as SetT } from '../types';
import Button from './ui/Button';
import CardCopyStack, { CardCopyDraft } from './ui/CardCopyStack';
import QuantityStepper from './ui/QuantityStepper';
import EbayIcon from './ui/EbayIcon';
import { ebaySearchUrl } from '../ebay';
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
  // Only needed for the eBay quick-link's search query - see SetDetail's
  // same field for why this is its own fetch rather than a single-set
  // lookup endpoint.
  const [setName, setSetName] = useState<string | null>(null);
  // card_id -> its owned copies (#108) is the *only* ownership state now -
  // quantity is just copies.length, there's no separate number to keep in
  // sync with it. A card's presence in this map (not the array's length)
  // is what decides whether it gets submitted at all: populated on load
  // from every card the server already returned any copies for, and added
  // lazily (starting from an empty array) the first time this session's
  // stepper touches a card the server never had a row for. An empty array
  // is submitted the same way a non-empty one would be - "own zero copies"
  // is a real, submittable state, not a reason to leave the card out.
  const [copiesByCard, setCopiesByCard] = useState<
    Record<string, CardCopyDraft[]>
  >({});
  // Which of a card's copies the stack is currently showing - only
  // meaningful once a card has 2+ copies (CardCopyStack ignores it
  // otherwise), but tracked for every card uniformly rather than only
  // ones past that threshold.
  const [activeIndexByCard, setActiveIndexByCard] = useState<
    Record<string, number>
  >({});
  // Which single card's stack is mid-cycle-animation, if any - a plain
  // cardID rather than a per-card boolean map, since only one stack can
  // reasonably be cycling at a time (one click, one card).
  const [shufflingCard, setShufflingCard] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  // Filtering only affects which rows render below - copiesByCard stays
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
      fetch('/api/sets'),
    ])
      .then(async ([cardsRes, ownedRes, allSetsRes]) => {
        if (cardsRes.status === 404) {
          setError('This set could not be found.');
          return;
        }
        if (cardsRes.status !== 200 || ownedRes.status !== 200) {
          setError('Could not load cards. Please try again.');
          return;
        }

        const owned: OwnedCardInput[] = await ownedRes.json();
        // Every returned card seeds copiesByCard, including one whose
        // copies array is empty (see the field's own doc comment above) -
        // it still needs to be "touched" so re-saving without changing it
        // submits that same empty list again, not silently drops the card
        // from the request entirely.
        const initialCopies: Record<string, CardCopyDraft[]> = {};
        for (const oc of owned) {
          initialCopies[oc.card_id] = (oc.copies ?? []).map((c) => ({
            id: c.id,
            price:
              c.price_paid_cents != null
                ? (c.price_paid_cents / 100).toFixed(2)
                : '',
          }));
        }
        setCopiesByCard(initialCopies);

        setCards(await cardsRes.json());

        // Not fatal if this one fails - the eBay link just falls back to
        // omitting the set name from its query rather than blocking the
        // whole page over a non-essential extra.
        if (allSetsRes.status === 200) {
          const allSets: SetT[] = await allSetsRes.json();
          setSetName(allSets.find((s) => s.id === setID)?.name ?? null);
        }
      })
      .catch(() => {
        setError('Could not reach the server. Please try again.');
      });
  }, [setID]);

  // Used by QuantityStepper's arrows and typing directly into its field
  // alike, same as pre-#108. Growing the array appends blank-priced
  // copies at the end - a new copy never guesses a price from the last
  // one, since "different prices for different copies" is the whole
  // reason #108 exists - and the stack jumps straight to the newest copy
  // so its price box is ready to type into immediately. Shrinking drops
  // from the end too (slice(0, target) keeps only the first target
  // entries) - the most-recently-added copies go first, not whichever the
  // stack happens to be showing, matching the backend's own LIFO default
  // for the legacy quantity-only path (reconcileOwnedCardCopies) so
  // reviewing an older copy via the cycle button and then decreasing
  // never deletes the one you were just looking at instead of the newest.
  const setQuantityDirect = (cardID: string, quantity: number) => {
    const target = Math.max(0, quantity);
    setCopiesByCard((prev) => {
      const current = prev[cardID] ?? [];
      const updated =
        target > current.length
          ? [
              ...current,
              ...Array.from({ length: target - current.length }, () => ({
                price: '',
              })),
            ]
          : current.slice(0, target);
      setActiveIndexByCard((prevActive) => ({
        ...prevActive,
        [cardID]: Math.max(0, updated.length - 1),
      }));
      return { ...prev, [cardID]: updated };
    });
  };

  const cycleCopy = (cardID: string) => {
    const count = (copiesByCard[cardID] ?? []).length;
    if (count <= 1 || shufflingCard) {
      return;
    }
    setShufflingCard(cardID);
    setTimeout(() => {
      setActiveIndexByCard((prev) => ({
        ...prev,
        [cardID]: ((prev[cardID] ?? 0) + 1) % count,
      }));
    }, 170);
    setTimeout(() => setShufflingCard(null), 420);
  };

  const setActiveCopyPrice = (cardID: string, value: string) => {
    setCopiesByCard((prev) => {
      const current = prev[cardID] ?? [];
      const active = activeIndexByCard[cardID] ?? current.length - 1;
      return {
        ...prev,
        [cardID]: current.map((c, i) =>
          i === active ? { ...c, price: value } : c,
        ),
      };
    });
  };

  // Normalizes the active copy's price text to a real "dollars.cents"
  // shape (e.g. "12" -> "12.00", "12.5" -> "12.50") once the user's done
  // editing, rather than fighting them mid-keystroke - re-formatting on
  // every change would clobber typing something like "12." before the
  // second decimal digit exists yet. Leaves an empty/invalid field alone -
  // blank means "no price entered," not "$0.00".
  const formatActiveCopyPriceOnBlur = (cardID: string) => {
    setCopiesByCard((prev) => {
      const current = prev[cardID] ?? [];
      const active = activeIndexByCard[cardID] ?? current.length - 1;
      const copy = current[active];
      if (!copy) {
        return prev;
      }
      const cents = dollarsToCents(copy.price);
      if (cents == null) {
        return prev;
      }
      return {
        ...prev,
        [cardID]: current.map((c, i) =>
          i === active ? { ...c, price: (cents / 100).toFixed(2) } : c,
        ),
      };
    });
  };

  // Onboards the set (idempotent either way) and, if any cards are passed,
  // records their ownership in the same submit. "Skip for now" calls this
  // with an empty list regardless of what's been edited or previously
  // owned - it never touches card ownership, only the set itself.
  const submit = (
    selectedCards: {
      card_id: string;
      copies: { id?: string; price_paid_cents?: number }[];
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

  // A card only gets submitted if it's actually in copiesByCard - one the
  // server already had a row for, or one this session's stepper touched.
  // A card nobody has ever interacted with (never owned, never clicked
  // this session) is left out entirely, same as before.
  const handleSave = () => {
    const toSubmit = cards ?? [];
    submit(
      toSubmit
        .filter((card) => card.id in copiesByCard)
        .map((card) => ({
          card_id: card.id,
          copies: (copiesByCard[card.id] ?? []).map((c) => ({
            id: c.id,
            price_paid_cents: dollarsToCents(c.price) ?? undefined,
          })),
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
    const isOwned = (copiesByCard[card.id]?.length ?? 0) > 0;
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
        <div className={styles.gridWrap}>
          <div className={styles.grid}>
            {visibleCards.map((card) => {
              const copies = copiesByCard[card.id] ?? [];
              const activeIndex = Math.min(
                activeIndexByCard[card.id] ?? 0,
                Math.max(0, copies.length - 1),
              );
              return (
                <div key={card.id} className={styles.tile}>
                  <CardCopyStack
                    cardId={card.id}
                    cardName={card.name}
                    copies={copies}
                    activeIndex={activeIndex}
                    shuffling={shufflingCard === card.id}
                    onCycle={() => cycleCopy(card.id)}
                    onPriceChange={(value) =>
                      setActiveCopyPrice(card.id, value)
                    }
                    onPriceBlur={() => formatActiveCopyPriceOnBlur(card.id)}
                    disabled={submitting}
                  />
                  <div className={styles.tileName}>{card.name}</div>
                  <div className={styles.tileCode}>{card.code}</div>
                  <span className={styles.rarityChip}>{card.rarity}</span>
                  <div className={styles.tileControls}>
                    <QuantityStepper
                      value={copies.length}
                      onChange={(next) => setQuantityDirect(card.id, next)}
                      ariaLabel={`quantity of ${card.name}`}
                      disabled={submitting}
                    />
                  </div>
                  <a
                    href={ebaySearchUrl(setName, card.code)}
                    target="_blank"
                    rel="noopener noreferrer"
                    className={styles.ebayLink}
                    aria-label={`Search eBay for ${card.name}`}
                  >
                    <EbayIcon />
                  </a>
                </div>
              );
            })}
          </div>
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

// undefined for "no price entered" (blank, or not a real number yet
// mid-typing) - cents (rounded, to sidestep float cruft like 12.1*100) for
// a real amount. A plain function (not a hook/component method) since it
// has no dependency on component state - just parses whatever string it's
// given.
const dollarsToCents = (raw: string): number | undefined => {
  if (!raw || raw.trim() === '') {
    return undefined;
  }
  const dollars = Number(raw);
  if (Number.isNaN(dollars)) {
    return undefined;
  }
  return Math.round(dollars * 100);
};

export default OnboardCards;

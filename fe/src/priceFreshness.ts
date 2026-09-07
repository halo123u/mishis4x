import type { Time } from './types';

// Turns a market_checked_at timestamp into the small "6m ago" / "2h ago" /
// "3d ago" caption from the refresh-mockup artifact (Option C) - amount and
// suffix are returned separately so the caller can style just the number
// (the "pop" accent), leaving "ago" in the muted caption color. Returns
// null when there's nothing to check yet at all (no card_price_sources row
// for this card) - callers should fall back to their own "Not tracked yet"
// text in that case rather than rendering an empty caption.
export type Freshness = { amount: string; suffix: string };

// Time is generated as an empty interface (typescriptify-golang-structs
// has no better mapping for Go's time.Time - see fe/src/types.ts) - it's
// actually always an RFC3339 string on the wire, so this is a real cast,
// not a lie, same quirk any other Time field in this codebase will hit.
export const formatFreshness = (checkedAt?: Time | null): Freshness | null => {
  if (!checkedAt) {
    return null;
  }

  const checkedMs = new Date(checkedAt as unknown as string).getTime();
  if (Number.isNaN(checkedMs)) {
    return null;
  }

  const diffMinutes = Math.max(0, Math.round((Date.now() - checkedMs) / 60000));

  if (diffMinutes < 1) {
    return { amount: 'just now', suffix: '' };
  }
  if (diffMinutes < 60) {
    return { amount: `${diffMinutes}m`, suffix: 'ago' };
  }

  const diffHours = Math.round(diffMinutes / 60);
  if (diffHours < 24) {
    return { amount: `${diffHours}h`, suffix: 'ago' };
  }

  const diffDays = Math.round(diffHours / 24);
  return { amount: `${diffDays}d`, suffix: 'ago' };
};

// "last seen $6.50 · 3d ago" - the small caption for a currently-
// out-of-stock card's last real price (api.Card's LastKnownMarket*
// fields), shared by every place that shows it (SetDetail's single-copy
// compare row, its "Missing" market pill, and CardCopyBrowseStack's
// per-copy comparison) so the wording never drifts between them. Falls
// back to just the price with no "· Nd ago" if checkedAt is somehow
// missing (shouldn't happen in practice - a last-known price always came
// from a real, timestamped card_price_history row - but a caller can't
// prove that from the type alone).
export const lastKnownPriceLabel = (
  priceCents: number,
  checkedAt?: Time | null,
): string => {
  const price = `last seen $${(priceCents / 100).toFixed(2)}`;
  const freshness = formatFreshness(checkedAt);
  if (!freshness) {
    return price;
  }
  return `${price} · ${freshness.amount}${freshness.suffix ? ` ${freshness.suffix}` : ''}`;
};

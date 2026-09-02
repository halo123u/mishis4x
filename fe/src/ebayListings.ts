import { EbayListing } from './types';

// The low-high spread across a card's currently fetched eBay listings -
// null when there's nothing to summarize (an empty result set). Computed
// client-side from whatever GET /api/cards/{id}/ebay-listings already
// returned, rather than the backend doing it - it's just the min/max of
// data already being displayed in full, not a derived statistic eBay's
// terms would treat as something needing separate consent (see
// [[ebay-api-license-terms]]).
export const priceRange = (
  listings: EbayListing[],
): { minCents: number; maxCents: number } | null => {
  if (listings.length === 0) {
    return null;
  }

  let minCents = listings[0].price_cents;
  let maxCents = listings[0].price_cents;
  for (const listing of listings) {
    if (listing.price_cents < minCents) minCents = listing.price_cents;
    if (listing.price_cents > maxCents) maxCents = listing.price_cents;
  }

  return { minCents, maxCents };
};

// Shared paid-vs-market comparison, used by both SetDetail's aggregate
// "Total paid vs. market×quantity" row and CardCopyBrowseStack's per-copy
// comparison (one owned copy's own price vs. the card's single-copy market
// price) - kept in one place so the exact wording/sign convention (under
// market reads as "good," over as "bad") never drifts between the two call
// sites now that there are two of them (see #108's follow-up: a card's
// market price only ever reflects one copy, so comparing it against a
// multi-copy aggregate needs the caller to already have scaled marketCents
// accordingly - this function itself doesn't know or care whether that
// scaling happened, it just diffs two cent amounts).
export type MarketDelta = {
  label: string;
  tone: 'good' | 'bad' | 'muted';
};

export const computeMarketDelta = (
  paidCents: number,
  marketCents: number,
): MarketDelta => {
  const deltaCents = paidCents - marketCents;
  if (deltaCents === 0) {
    return { label: 'At market price', tone: 'muted' };
  }
  const under = deltaCents < 0;
  const amount = (Math.abs(deltaCents) / 100).toFixed(2);
  return {
    label: `${under ? '▼' : '▲'} $${amount} ${under ? 'under' : 'over'} market`,
    tone: under ? 'good' : 'bad',
  };
};

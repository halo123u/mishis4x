// RRR and CR are genuinely landscape-printed cards in the real catalog -
// not a scan-orientation bug, the physical cards themselves are wider
// than tall, unlike every other rarity's standard 63mm x 88mm portrait
// card. Centralized here (rather than each caller re-checking rarity
// strings) so CardThumbnail's whole call chain - CardThumbnail itself,
// CardCopyStack/CardCopyBrowseStack's stack peek layers, and anything
// else that ever needs to know a card's on-screen shape - agrees on
// exactly which rarities this applies to.
const landscapeRarities = new Set(['RRR', 'CR']);

export const isLandscapeCard = (rarity: string): boolean =>
  landscapeRarities.has(rarity);

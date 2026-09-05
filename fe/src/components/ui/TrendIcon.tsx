// A plain ascending-bars glyph (hand-drawn path, not a brand mark like
// EbayIcon) - matches RefreshIcon's own line-icon style so the two sit
// naturally side by side in a tile's freshness row.
const TrendIcon = () => (
  <svg
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    strokeLinecap="round"
    strokeLinejoin="round"
    width="14"
    height="14"
    aria-hidden="true"
  >
    <line x1="5" y1="20" x2="5" y2="13" />
    <line x1="12" y1="20" x2="12" y2="8" />
    <line x1="19" y1="20" x2="19" y2="4" />
  </svg>
);

export default TrendIcon;

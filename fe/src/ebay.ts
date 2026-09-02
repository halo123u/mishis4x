// _sop=15 sorts by price+shipping lowest-first - the useful default for
// "what's this actually going for", not eBay's default relevance sort.
export const ebaySearchUrlForQuery = (query: string): string =>
  `https://www.ebay.com/sch/i.html?_nkw=${encodeURIComponent(query)}&_sop=15`;

// eBay's own listing titles/search results key off the short card number
// (e.g. "086S"), not the full catalog code with its set prefix
// ("BRD/W139-086S") - searching with the full code returns few or no
// results, since sellers don't title listings that way. Everything after
// the code's last "-" is that short number; take just that, paired with
// the set's plain name, to actually find results. Same query-building
// logic as be/ebay's SearchQuery - kept as textual twins rather than one
// calling the other, since one's Go and one's TS.
export const ebaySearchUrl = (setName: string | null, code: string): string => {
  const shortCode = code.split('-').pop() ?? code;
  // Still a usable (if less precise) search without the set name - e.g.
  // if the set list fetch that supplies it happened to fail - rather than
  // hiding the link entirely over a non-essential extra.
  const query = setName ? `${setName} ${shortCode}` : shortCode;
  return ebaySearchUrlForQuery(query);
};

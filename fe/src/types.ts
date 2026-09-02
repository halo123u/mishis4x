/* Do not change, this code is generated from Golang structs */

export interface User {
  id: number;
  username: string;
  status: string;
}
export interface GlobalData {
  user: User;
  ebay_listings_enabled: boolean;
}
export interface Time {}
export interface Set {
  id: string;
  name: string;
  card_count: number;
  release_date?: Time;
  status: string;
}
export interface Card {
  id: string;
  set_id: string;
  name: string;
  code: string;
  rarity: string;
  market_price_cents?: number;
  market_checked_at?: Time;
  market_url?: string;
}
export interface AddOwnedSetInput {
  set_id: string;
}
export interface OwnedCardInput {
  card_id: string;
  quantity: number;
  price_paid_cents?: number;
}
export interface SetOwnedCardsInput {
  cards: OwnedCardInput[];
}
export interface EbayListing {
  item_id: string;
  title: string;
  price_cents: number;
  condition: string;
  seller_username: string;
  seller_feedback_percentage: string;
  item_web_url: string;
  image_url: string;
}
export interface EbayListingsResponse {
  query: string;
  listings: EbayListing[];
}

/* Do not change, this code is generated from Golang structs */

export interface User {
  id: number;
  username: string;
  status: string;
  is_admin: boolean;
}
export interface GlobalData {
  user: User;
  ebay_listings_enabled: boolean;
  price_trends_enabled: boolean;
  model_viewer_enabled: boolean;
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
  last_known_market_price_cents?: number;
  last_known_market_checked_at?: Time;
  character_model_char_code?: string;
}
export interface AddOwnedSetInput {
  set_id: string;
}
export interface OwnedCardCopyInput {
  id?: string;
  price_paid_cents?: number;
}
export interface OwnedCardInput {
  card_id: string;
  quantity: number;
  price_paid_cents?: number;
  copies?: OwnedCardCopyInput[];
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
export interface DailyPricePoint {
  date: string;
  price_cents: number;
}
export interface CardPriceTrend {
  card_id: string;
  daily_prices: DailyPricePoint[];
  change_cents: number;
  change_percent: number;
}
export interface AdminInviteRequest {
  id: number;
  email_address: string;
  created_at: Time;
}
export interface SetCardCharacterModelInput {
  char_code?: string;
}
export interface ModelDisplayState {
  char_code: string;
  flip?: string;
  pepper: boolean;
}
export interface ModelDisplayStatus {
  connected: boolean;
}

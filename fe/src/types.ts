/* Do not change, this code is generated from Golang structs */

export interface User {
  id: number;
  username: string;
  status: string;
}
export interface GlobalData {
  user: User;
  collection_access: boolean;
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

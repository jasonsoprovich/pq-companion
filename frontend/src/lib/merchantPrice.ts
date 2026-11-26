// Merchant sell-price estimate — a port of Project Quarm's server code
// (EQMacEmu zone/client.cpp Client::CalcPriceMod and client_packet.cpp
// Handle_OP_ShopPlayerSell). A merchant pays, per item:
//
//   round(item.price × rate) × quantity   (charged items: quantity 1)
//
// where `rate` depends on the seller's Charisma, their faction with that
// merchant, and the merchant's greed. This app can't know which merchant
// you'll sell to, so the estimate assumes a normal (greed 0) merchant —
// ~94% of Quarm's merchants — and lets the caller pick the faction tier.
// Race, deity, class and illusions only matter through that faction tier.

export type MerchantFaction = 'amiable' | 'indifferent' | 'apprehensive'

// Highest rate a merchant ever pays: 1 / 1.05 of base price.
export const MAX_SELL_RATE = 1 / 1.05

// sellRate returns the fraction of base price a greed-0 merchant pays a
// character with `cha` Charisma at the given faction tier. Amiable or better
// counts as +11 Charisma and uses the indifferent curve.
export function sellRate(cha: number, faction: MerchantFaction): number {
  let c = cha
  if (faction === 'amiable') c += 11
  let rate: number
  if (faction !== 'apprehensive') {
    if (c > 75) rate = 1 - (115 - c) * 0.004
    else if (c > 60) rate = 1 / 1.25
    else rate = 1 / (1 - (c - 120) / 220)
  } else {
    if (c > 75) rate = (100 - (145 - c) / 2.8) / 100
    else if (c > 60) rate = 1 / 1.4
    else rate = 1 / (1 + (143.574 - c) / 196.434)
  }
  return Math.min(rate, MAX_SELL_RATE)
}

// payout is what a merchant pays for `qty` of an item with base `price`
// (copper) at `rate`. The server rounds per unit, then multiplies.
export function payout(price: number, qty: number, rate: number): number {
  return Math.floor(price * rate + 0.5) * qty
}

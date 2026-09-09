// Package trader powers the (developer-tab) Bazaar Trader Tracker: it infers
// what a trader character sold while parked in the bazaar by diffing successive
// inventory exports.
//
// The bazaar has no log: entering /trader boots the client (so the normal
// export-on-camp never fires) and nothing is written when an item sells. The
// only durable signal is the inventory itself. So the workflow is:
//
//  1. /output inventory BEFORE entering trader mode (snapshot A)
//  2. park in /trader, items sell over time
//  3. log back in, /output inventory AGAIN (snapshot B)
//
// Items that left the character's Trader's Satchels between A and B were almost
// certainly sold. The BZR_<Char>.ini price file is NOT a sale signal — items
// are never removed from it — it's only a price reference (every price the
// trader has ever set), used here to annotate each inferred sale and estimate
// revenue. The on-person coin delta reconciles the total.
//
// This is best-effort by nature (it can't tell a sale from a manual delete, a
// vendor sale, or a give-away), so every inferred session carries caveats.
package trader

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/zeal"
)

// TraderSatchelItemID is the item ID of a Trader's Satchel on Project Quarm.
// Only items inside a Trader's Satchel can be sold in the bazaar, so satchel
// contents are the inference surface. Name matching is also used as a fallback
// in case other satchel sizes/IDs exist.
const TraderSatchelItemID = 17899

// PricedItem is one entry from a trader's BZR_<Char>.ini [ItemToSell] section.
type PricedItem struct {
	Name  string `json:"name"`
	Price int64  `json:"price"` // copper; 0 means listed but not currently for sale
}

// BZRListing is the parsed price database for one trader character. It is
// append-only in game (selling an item does not remove it) so it is a price
// reference only, never a "what is for sale right now" list.
type BZRListing struct {
	Character string       `json:"character"`
	Items     []PricedItem `json:"items"`
}

// priceOf returns the listed price for an item name (case-insensitive) and
// whether the item appears in the BZR file at all.
func (l *BZRListing) priceOf(name string) (int64, bool) {
	if l == nil {
		return 0, false
	}
	want := strings.ToLower(strings.TrimSpace(name))
	for _, it := range l.Items {
		if strings.ToLower(it.Name) == want {
			return it.Price, true
		}
	}
	return 0, false
}

// SatchelItem is one item occupying a Trader's Satchel slot at snapshot time.
type SatchelItem struct {
	Bag    int    `json:"bag"`  // which bag holds the satchel (1-based)
	Slot   int    `json:"slot"` // slot within the satchel (1-based)
	ItemID int    `json:"item_id"`
	Name   string `json:"name"`
	Count  int    `json:"count"`
	// Vault is true when the satchel sits in a Bank slot rather than the
	// inventory. Bank Trader's Satchels do not list on the bazaar bar, but
	// traders routinely shuffle unsold stock between an inventory satchel and a
	// bank satchel on relog. Tracking both locations means such a move nets to
	// zero in the diff instead of reading as a sale + a restock. Consumers that
	// care about "what is on the bar" (the listings stock count, snapshot item
	// counts) filter these out; the sale inference counts them.
	Vault bool `json:"vault,omitempty"`
}

// OnBar reports whether the item is in an inventory (bazaar-listed) Trader's
// Satchel rather than a bank one.
func (it SatchelItem) OnBar() bool { return !it.Vault }

// Snapshot is the trader-relevant state captured from one inventory/quarmy
// export: the contents of every Trader's Satchel (inventory and bank) plus the
// character's coin.
type Snapshot struct {
	Character      string        `json:"character"`
	TakenAt        time.Time     `json:"taken_at"`
	Satchel        []SatchelItem `json:"satchel"`
	OnPersonCopper int64         `json:"on_person_copper"`
	BankCopper     int64         `json:"bank_copper"`
	SourcePath     string        `json:"source_path"`
}

// Fingerprint is a content hash used to skip storing a snapshot that is
// identical to the previous one. Two exports with the same satchel contents and
// coin produce the same fingerprint regardless of file mod time.
func (s *Snapshot) Fingerprint() string {
	if s == nil {
		return ""
	}
	parts := make([]string, 0, len(s.Satchel)+1)
	for _, it := range s.Satchel {
		parts = append(parts, fmt.Sprintf("%d:%d", it.ItemID, it.Count))
	}
	sort.Strings(parts)
	parts = append(parts, fmt.Sprintf("coin:%d:%d", s.OnPersonCopper, s.BankCopper))
	return strings.Join(parts, "|")
}

// satchelByItem aggregates satchel counts by item ID (an item may occupy
// several slots/bags). The returned name map keeps a display name per item ID.
func (s *Snapshot) satchelByItem() (counts map[int]int, names map[int]string) {
	counts = make(map[int]int)
	names = make(map[int]string)
	for _, it := range s.Satchel {
		counts[it.ItemID] += it.Count
		if _, ok := names[it.ItemID]; !ok {
			names[it.ItemID] = it.Name
		}
	}
	return counts, names
}

// SoldItem is one item whose satchel count dropped between two snapshots
// (Qty > 0). UnitPrice/LineTotal come from the BZR file; Listed is false when
// the item has no BZR price (still reported, but unpriced).
type SoldItem struct {
	ItemID    int    `json:"item_id"`
	Name      string `json:"name"`
	Qty       int    `json:"qty"`
	UnitPrice int64  `json:"unit_price"`
	LineTotal int64  `json:"line_total"`
	Listed    bool   `json:"listed"`
	Icon      int    `json:"icon,omitempty"`
}

// Session is the inferred outcome of diffing two consecutive snapshots.
type Session struct {
	Character        string     `json:"character"`
	FromTime         time.Time  `json:"from_time"`
	ToTime           time.Time  `json:"to_time"`
	Sold             []SoldItem `json:"sold"`              // satchel count decreased
	Restocked        []SoldItem `json:"restocked"`         // satchel count increased (Qty = amount added)
	EstimatedRevenue int64      `json:"estimated_revenue"` // sum of listed line totals
	OnPersonDelta    int64      `json:"on_person_delta"`
	TotalCoinDelta   int64      `json:"total_coin_delta"`
	CoinGained       int64      `json:"coin_gained"` // best estimate of coin taken in, bank-aware
	Reconciles       bool       `json:"reconciles"`  // estimated revenue ≈ coin gained
	Suspect          bool       `json:"suspect"`     // likely diffing a stale "before" snapshot
	Caveats          []string   `json:"caveats"`
}

// reconcileToleranceCopper allows for rounding / minor incidental coin movement
// when deciding whether estimated revenue matches the coin gained.
const reconcileToleranceCopper = 1000 // 1 platinum

// staleSnapshotGap is how far apart two snapshots must be before the session is
// flagged as possibly diffing against a leftover "before" export from an earlier
// trip rather than a deliberate pre-park capture. A genuine park rarely runs
// past two days between inventory exports.
const staleSnapshotGap = 48 * time.Hour

// InferSales diffs prev (older) against next (newer) and returns the inferred
// sale session. listing may be nil (prices simply come back as 0/unlisted).
func InferSales(prev, next *Snapshot, listing *BZRListing) *Session {
	prevCounts, prevNames := prev.satchelByItem()
	nextCounts, nextNames := next.satchelByItem()

	sess := &Session{
		Character:     next.Character,
		FromTime:      prev.TakenAt,
		ToTime:        next.TakenAt,
		Sold:          []SoldItem{},
		Restocked:     []SoldItem{},
		OnPersonDelta: next.OnPersonCopper - prev.OnPersonCopper,
		TotalCoinDelta: (next.OnPersonCopper + next.BankCopper) -
			(prev.OnPersonCopper + prev.BankCopper),
	}

	// Items present before: a drop in count is a candidate sale.
	for id, before := range prevCounts {
		after := nextCounts[id]
		if after >= before {
			continue
		}
		qty := before - after
		name := prevNames[id]
		price, listed := listing.priceOf(name)
		line := int64(0)
		if listed && price > 0 {
			line = price * int64(qty)
			sess.EstimatedRevenue += line
		}
		sess.Sold = append(sess.Sold, SoldItem{
			ItemID:    id,
			Name:      name,
			Qty:       qty,
			UnitPrice: price,
			LineTotal: line,
			Listed:    listed && price > 0,
		})
	}

	// Items whose count rose (or newly appeared): a restock, not a sale.
	for id, after := range nextCounts {
		before := prevCounts[id]
		if after <= before {
			continue
		}
		price, listed := listing.priceOf(nextNames[id])
		sess.Restocked = append(sess.Restocked, SoldItem{
			ItemID:    id,
			Name:      nextNames[id],
			Qty:       after - before,
			UnitPrice: price,
			Listed:    listed && price > 0,
		})
	}

	sort.Slice(sess.Sold, func(i, j int) bool {
		if sess.Sold[i].LineTotal != sess.Sold[j].LineTotal {
			return sess.Sold[i].LineTotal > sess.Sold[j].LineTotal
		}
		return sess.Sold[i].Name < sess.Sold[j].Name
	})
	sort.Slice(sess.Restocked, func(i, j int) bool {
		return sess.Restocked[i].Name < sess.Restocked[j].Name
	})

	// Reconciliation: bazaar sales pay the trader directly, so the coin they
	// took in should be roughly the estimated revenue. A trader who logs back
	// in and banks their earnings before the next /output inventory shows a
	// negative on-person delta even though sales happened — the bank-inclusive
	// total delta survives that. Use whichever is larger so a banking trader
	// still reconciles; on-person-only would flag every such session "coin off".
	sess.CoinGained = sess.TotalCoinDelta
	if sess.OnPersonDelta > sess.CoinGained {
		sess.CoinGained = sess.OnPersonDelta
	}
	diff := sess.CoinGained - sess.EstimatedRevenue
	if diff < 0 {
		diff = -diff
	}
	sess.Reconciles = sess.EstimatedRevenue > 0 && diff <= reconcileToleranceCopper

	// Stale-"before" heuristic. A leftover export from a previous trip diffs
	// against a fresh one as a big pile of "sold" items with no matching coin
	// and (often) no listed prices. Flag it either when the snapshots are days
	// apart, or when items left but nothing priced moved and no coin came in
	// anywhere — both point at a bad pairing rather than a real session.
	gap := sess.ToTime.Sub(sess.FromTime)
	noCoin := sess.OnPersonDelta <= 0 && sess.TotalCoinDelta <= 0
	sess.Suspect = gap > staleSnapshotGap ||
		(len(sess.Sold) > 0 && sess.EstimatedRevenue == 0 && noCoin)

	sess.Caveats = sessionCaveats(sess)
	return sess
}

// sessionCaveats returns human-readable warnings about the limits of the
// inference, tailored to what the diff actually found.
func sessionCaveats(s *Session) []string {
	caveats := []string{
		"Inferred from inventory differences — not a real sales log.",
		"A satchel item that left can't be told apart from a manual delete, give-away, or vendor sale.",
	}
	if s.Suspect {
		reason := "no coin or priced items moved"
		if s.ToTime.Sub(s.FromTime) > staleSnapshotGap {
			reason = fmt.Sprintf("they're %s apart", humanGap(s.ToTime.Sub(s.FromTime)))
		}
		caveats = append(caveats, fmt.Sprintf(
			"This looks like it may be diffing against a stale \"before\" snapshot (%s). If the earlier snapshot is a leftover export from a previous trip, delete it on the Snapshots tab and capture a fresh one before parking.",
			reason))
	}
	if s.OnPersonDelta < 0 && s.CoinGained > 0 {
		caveats = append(caveats,
			"On-person coin fell but total coin (including the bank) rose — earnings were banked; reconciled against the total.")
	}
	if s.EstimatedRevenue > 0 && !s.Reconciles {
		if s.CoinGained < s.EstimatedRevenue {
			caveats = append(caveats,
				"Coin taken in (including the bank) is less than the listed value of what left — items may have sold below list, been given away, or restock was bought from the bank.")
		} else {
			caveats = append(caveats,
				"Coin taken in exceeds the listed value of what left — some items that sold have no BZR price, or the listed prices are stale.")
		}
	}
	hasUnlisted := false
	for _, it := range s.Sold {
		if !it.Listed {
			hasUnlisted = true
			break
		}
	}
	if hasUnlisted {
		caveats = append(caveats,
			"Some items that left a satchel have no price in the BZR file, so they're unpriced here.")
	}
	return caveats
}

// humanGap renders a snapshot-to-snapshot gap for a caveat: whole days past two
// days, otherwise whole hours.
func humanGap(d time.Duration) string {
	if d >= 48*time.Hour {
		return fmt.Sprintf("%d days", int(d.Hours())/24)
	}
	return fmt.Sprintf("%d hours", int(d.Hours()))
}

// --- Parsing -------------------------------------------------------------

var (
	bagRe  = regexp.MustCompile(`^(General|Bank)(\d+)$`)
	slotRe = regexp.MustCompile(`^(General|Bank)(\d+)-Slot(\d+)$`)
)

// isTraderSatchel reports whether an inventory bag row is a Trader's Satchel,
// by item ID or by name (covers any future satchel variants).
func isTraderSatchel(id int, name string) bool {
	if id == TraderSatchelItemID {
		return true
	}
	return strings.Contains(strings.ToLower(name), "trader's satchel") ||
		strings.Contains(strings.ToLower(name), "trader satchel")
}

// ParseSnapshot extracts the trader-relevant state (Trader's Satchel contents +
// coin) from a Zeal inventory or quarmy export. Both file types share the same
// tab-delimited inventory section, so this parser handles either; non-inventory
// lines (stats header, AA rows) simply don't match and are ignored.
//
// It deliberately does NOT reuse zeal.ParseInventory: that parser coerces a
// count of 0 to 1 (fine for item display, wrong for the General-Coin row, which
// is legitimately 0 when the trader carries no coin).
func ParseSnapshot(path, character string) (*Snapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	snap := &Snapshot{
		Character:  character,
		TakenAt:    info.ModTime(),
		Satchel:    []SatchelItem{},
		SourcePath: path,
	}

	// First gather raw rows so we can resolve which bags are satchels before
	// reading their slot children (bag row precedes its slots, but a two-pass
	// approach is robust to ordering). Both General* (on the bar) and Bank*
	// (vault storage) Trader's Satchels are tracked.
	type row struct {
		area  string // "General" or "Bank"
		bag   int
		slot  int
		name  string
		id    int
		count int
	}
	var rows []row
	satchelBags := make(map[string]bool) // "General2" / "Bank5" -> is a trader satchel

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 4 {
			continue
		}
		loc := strings.TrimSpace(parts[0])
		name := strings.TrimSpace(parts[1])
		id, err := strconv.Atoi(strings.TrimSpace(parts[2]))
		if err != nil {
			continue // header rows ("Location"), stats rows, etc.
		}
		count, _ := strconv.Atoi(strings.TrimSpace(parts[3]))

		switch loc {
		case "General-Coin":
			snap.OnPersonCopper = int64(count)
			continue
		case "Bank-Coin":
			snap.BankCopper = int64(count)
			continue
		}

		if m := bagRe.FindStringSubmatch(loc); m != nil {
			if isTraderSatchel(id, name) {
				satchelBags[m[1]+m[2]] = true
			}
			continue
		}
		if m := slotRe.FindStringSubmatch(loc); m != nil {
			bag, _ := strconv.Atoi(m[2])
			slot, _ := strconv.Atoi(m[3])
			rows = append(rows, row{
				area: m[1], bag: bag, slot: slot,
				name: name, id: id, count: count,
			})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	for _, r := range rows {
		if !satchelBags[fmt.Sprintf("%s%d", r.area, r.bag)] {
			continue // slot belongs to a normal bag, not a Trader's Satchel
		}
		if r.id == 0 || strings.EqualFold(r.name, "Empty") {
			continue
		}
		cnt := r.count
		if cnt <= 0 {
			cnt = 1
		}
		snap.Satchel = append(snap.Satchel, SatchelItem{
			Bag:    r.bag,
			Slot:   r.slot,
			ItemID: r.id,
			Name:   r.name,
			Count:  cnt,
			Vault:  r.area == "Bank",
		})
	}

	sort.Slice(snap.Satchel, func(i, j int) bool {
		a, b := snap.Satchel[i], snap.Satchel[j]
		if a.Vault != b.Vault {
			return !a.Vault // on-bar satchels first
		}
		if a.Bag != b.Bag {
			return a.Bag < b.Bag
		}
		return a.Slot < b.Slot
	})
	return snap, nil
}

// ParseBZR reads a BZR_<Char>.ini trader price file. It returns the items from
// the [ItemToSell] section in file order, including price-0 (not-for-sale)
// entries. A missing file is reported via os.IsNotExist on the returned error.
func ParseBZR(path, character string) (*BZRListing, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := &BZRListing{Character: character, Items: []PricedItem{}}
	inSection := false
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inSection = strings.EqualFold(line, "[ItemToSell]")
			continue
		}
		if !inSection {
			continue
		}
		eq := strings.LastIndexByte(line, '=')
		if eq < 0 {
			continue
		}
		name := strings.TrimSpace(line[:eq])
		price, err := strconv.ParseInt(strings.TrimSpace(line[eq+1:]), 10, 64)
		if err != nil || name == "" {
			continue
		}
		out.Items = append(out.Items, PricedItem{Name: name, Price: price})
	}
	return out, scanner.Err()
}

// --- File discovery ------------------------------------------------------

var bzrNameRe = regexp.MustCompile(`(?i)^BZR_(.+?)(_pq\.proj)?\.ini$`)

// FindBZRCharacters scans an EQ directory for BZR_<Char>.ini files and returns
// the trader character names (deduplicated, in sorted order). Both the
// format-0 (BZR_<Char>.ini) and format-1 (BZR_<Char>_pq.proj.ini) names match.
func FindBZRCharacters(eqPath string) ([]string, error) {
	if eqPath == "" {
		return nil, nil
	}
	matches, err := filepath.Glob(filepath.Join(eqPath, "BZR_*.ini"))
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var names []string
	for _, p := range matches {
		m := bzrNameRe.FindStringSubmatch(filepath.Base(p))
		if m == nil {
			continue
		}
		name := m[1]
		if name == "" || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// FindBZRFile returns the path to a character's BZR price file, preferring the
// most recently modified when both naming formats exist. Returns "" if none.
func FindBZRFile(eqPath, character string) string {
	return newestExisting(
		filepath.Join(eqPath, fmt.Sprintf("BZR_%s.ini", character)),
		filepath.Join(eqPath, fmt.Sprintf("BZR_%s_pq.proj.ini", character)),
	)
}

// FindExportFile returns the newest trader-relevant export for a character —
// either the -Inventory or -Quarmy file (both carry satchel contents + coin),
// whichever was written most recently. Returns "" if neither exists.
func FindExportFile(eqPath, character string) string {
	inv := zeal.FindInventoryFile(eqPath, character)
	quarmy := zeal.FindQuarmyFile(eqPath, character)
	return newestExisting(inv, quarmy)
}

// newestExisting returns the path with the latest mod time among those that
// exist on disk, or "" if none do (empty paths are skipped).
func newestExisting(paths ...string) string {
	best := ""
	var bestMod time.Time
	for _, p := range paths {
		if p == "" {
			continue
		}
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if best == "" || info.ModTime().After(bestMod) {
			best, bestMod = p, info.ModTime()
		}
	}
	return best
}

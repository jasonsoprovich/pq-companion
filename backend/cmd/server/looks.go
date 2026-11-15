package main

import (
	"fmt"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/players"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zealpipe"
)

// lookOf turns a Zeal raid or group member's appearance into the look the
// players store keeps; ok is false when there's none.
func lookOf(a *zealpipe.Appearance) (players.Look, bool) {
	if a == nil || a.Race <= 0 {
		return players.Look{}, false
	}
	l := players.Look{
		Race: a.Race, Gender: a.Gender, Texture: a.Texture, Size: a.Size, Face: a.Face,
		HairStyle: a.HairStyle, HairColor: a.HairColor,
		BeardStyle: a.BeardStyle, BeardColor: a.BeardColor,
	}
	for i := 0; i < len(l.Mats) && i < len(a.Equipment); i++ {
		l.Mats[i] = a.Equipment[i]
	}
	item := func(i int) string {
		if i < len(a.Equipment) && a.Equipment[i] > 0 {
			return fmt.Sprintf("IT%d", a.Equipment[i])
		}
		return ""
	}
	l.Primary, l.Secondary = item(7), item(8)
	for i := 0; i < len(l.Tints) && i < len(a.EquipmentColors); i++ {
		l.Tints[i] = a.EquipmentColors[i] & 0xffffff
	}
	return l, true
}

// recordLooks keeps how each member in a raid or group frame looks.
func recordLooks(store *players.Store, members map[string]*zealpipe.Appearance) {
	if store == nil {
		return
	}
	now := time.Now()
	for name, a := range members {
		if l, ok := lookOf(a); ok {
			_ = store.SetLook(name, l, now)
		}
	}
}

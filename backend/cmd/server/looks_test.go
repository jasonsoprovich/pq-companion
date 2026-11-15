package main

import (
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/players"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zealpipe"
)

func TestLookOfRaidAppearance(t *testing.T) {
	members, err := zealpipe.DecodeRaid(`[{"name":"Bela","level":60,"class":4,"group":"1","rank":"",
		"spawn_id":12,"loc":{"x":1,"y":2,"z":3},"heading":10,
		"appearance":{"race":6,"gender":1,"texture":255,"size":5.0,"face":3,"hair_style":1,
		"hair_color":12,"beard_style":255,"beard_color":255,
		"equipment":[3,3,3,3,3,3,3,38,0],
		"equipment_colors":[0,4281479780,4281479780,0,0,0,0]}},
		{"name":"Away","level":60,"class":1,"group":"2","rank":""}]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 || members[1].Appearance != nil {
		t.Fatalf("members %+v", members)
	}
	got, ok := lookOf(members[0].Appearance)
	want := players.Look{
		Race: 6, Gender: 1, Texture: 255, Size: 5, Face: 3, HairStyle: 1, HairColor: 12,
		BeardStyle: 255, BeardColor: 255, Mats: [7]int{3, 3, 3, 3, 3, 3, 3},
		Tints: [7]uint32{0, 0x323264, 0x323264}, Primary: "IT38",
	}
	if !ok || got != want {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
	if _, ok := lookOf(nil); ok {
		t.Error("look from no appearance")
	}
}

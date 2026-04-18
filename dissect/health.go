package dissect

import (
	"encoding/binary"

	"github.com/rs/zerolog/log"
)

// healthTag is the 4-byte field identifier carrying player HP inside the
// 0x23-prefixed per-entity property stream. Discovered by RE on
// Y11S1_Alpha03 (code 9625601) replays — 10 entities per round, uint32
// values in 0..130, drops on damage, reaches 0 at death. Not present in
// redraskal/r6-dissect upstream.
//
// The tag is wrapped by the standard entity-property envelope:
//
//	23 <4B entity_ref> 00 00 00 00 25 26 76 C9 04 <uint32 LE hp>
//
// We listen on the bare 4-byte tag and verify the envelope inside the
// handler so random byte sequences that happen to match the tag are
// discarded instead of producing garbage HP readings.
var healthTag = []byte{0x25, 0x26, 0x76, 0xC9}

// healthSample records the observed HP window for one entity over the
// round. Max is the starting HP (100 default, 110/125/130 for armored
// or Rook-buffed operators); Min is the lowest HP seen after the entity
// first showed up alive. damage_taken = Max - Min.
//
// Timeline is the full ordered list of (offset, hp) samples. Used to
// look up HP at a specific kill offset for per-blow damage accuracy
// (distinguishes solo-frags from kill-stealers on pre-damaged victims).
//
// The stream emits HP=0 on every entity *before* the round actually
// starts (pre-spawn slot state). Treating that as "min HP = 0" would
// flag every survivor as having taken full damage. Alive tracks whether
// we've seen a non-zero value yet; zeros before that are ignored, zeros
// after (i.e. real deaths) are recorded.
type healthSample struct {
	Min      uint32
	Max      uint32
	Alive    bool
	Set      bool
	Timeline []healthPoint
}

type healthPoint struct {
	Offset int
	HP     uint32
}

// hpBefore returns the last HP value recorded for this entity at a
// file offset strictly less than `offset`. Returns 0, false if no
// earlier sample exists.
func (s healthSample) hpBefore(offset int) (uint32, bool) {
	if len(s.Timeline) == 0 {
		return 0, false
	}
	// Timeline is append-only in offset order (listeners fire in
	// sorted offset order by reader.Read), so a reverse linear scan
	// finds the last sample before `offset` quickly. Binary search
	// would be slightly cleaner but this runs ~10x per player per
	// round — linear is fine.
	for i := len(s.Timeline) - 1; i >= 0; i-- {
		if s.Timeline[i].Offset < offset {
			return s.Timeline[i].HP, true
		}
	}
	return 0, false
}

// The HP stream's 4-byte entity ref is NOT the same value as the
// player's DissectID. Observation across test replays: each player's
// HP entity has an ID slightly *less* than their DissectID (offsets of
// 1..5 depending on how many gadgets/objects were allocated between
// them at round start). Mapping is recovered in PlayerStats() by
// pairing each HP entity with the nearest DissectID that's ≥ the
// entity. So readHealth only needs to record samples keyed by the raw
// entity ID.
func readHealth(r *Reader) error {
	// On entry, r.offset sits just past the 4-byte tag. The envelope
	// we want to validate sits in r.b[r.offset-13 : r.offset]:
	//
	//   r.offset-13        : 0x23 (record start)
	//   r.offset-12..-9    : 4-byte entity ref
	//   r.offset-8..-5     : 00 00 00 00 (zero pad)
	//   r.offset-4..-1     : 25 26 76 C9 (the tag itself)
	if r.offset < 13 || r.offset+5 > len(r.b) {
		return nil
	}
	if r.b[r.offset-13] != 0x23 {
		return nil
	}
	for i := r.offset - 8; i < r.offset-4; i++ {
		if r.b[i] != 0 {
			return nil
		}
	}
	entity := binary.LittleEndian.Uint32(r.b[r.offset-12 : r.offset-8])

	if r.b[r.offset] != 0x04 {
		return nil
	}
	hp := binary.LittleEndian.Uint32(r.b[r.offset+1 : r.offset+5])
	r.offset += 5

	if r.health == nil {
		r.health = make(map[uint32]healthSample)
	}
	s := r.health[entity]
	if hp == 0 && !s.Alive {
		// Pre-spawn slot state — ignore.
		return nil
	}
	if hp > 0 {
		s.Alive = true
	}
	if !s.Set {
		s.Min = hp
		s.Max = hp
		s.Set = true
	} else {
		if hp < s.Min {
			s.Min = hp
		}
		if hp > s.Max {
			s.Max = hp
		}
	}
	// Sample offset is the position AFTER the 5 bytes we just consumed
	// (size + uint32). Recording the pre-consume offset (r.offset - 5)
	// keeps samples aligned with "what HP was visible before this
	// read point" — matches kill offsets we'll compare against.
	s.Timeline = append(s.Timeline, healthPoint{Offset: r.offset - 5, HP: hp})
	r.health[entity] = s

	log.Debug().
		Uint32("entity", entity).
		Uint32("hp", hp).
		Msg("hp_update")
	return nil
}

// playerForEntity returns the player index whose DissectID is the
// smallest value ≥ entity, within a small delta. Returns -1 if no
// player qualifies. Used by every 23-prefix per-entity stat tracker —
// HP, score, assists, etc.
//
// Rule comes from the observation that each player has a cluster of
// satellite entities (character, scoreboard row, gadgets) allocated
// just below their DissectID at round start. "Nearest higher DissectID"
// picks the right player for any of them in every replay tested.
func (r *Reader) playerForEntity(entity uint32) (int, uint32) {
	best := -1
	var bestDelta uint32
	for i, p := range r.Header.Players {
		if len(p.DissectID) != 4 {
			continue
		}
		pid := binary.LittleEndian.Uint32(p.DissectID)
		if pid < entity {
			continue
		}
		d := pid - entity
		if best == -1 || d < bestDelta {
			best = i
			bestDelta = d
		}
	}
	if best == -1 || bestDelta > 64 {
		return -1, 0
	}
	return best, bestDelta
}

// healthByPlayer pairs each tracked HP entity with the player whose
// DissectID is the smallest value ≥ the entity ID. Returns a map from
// player index → aggregated health sample. Entities that don't pair to
// any player (gadgets, destructibles) are dropped.
func (r *Reader) healthByPlayer() map[int]healthSample {
	out := make(map[int]healthSample)
	if r.health == nil || len(r.Header.Players) == 0 {
		return out
	}
	for ent, s := range r.health {
		idx, _ := r.playerForEntity(ent)
		if idx < 0 {
			continue
		}
		// If two entities both map to the same player, keep the one
		// with the wider HP window — the real character entity
		// accumulates many samples across the round.
		if prev, ok := out[idx]; ok {
			if (s.Max - s.Min) <= (prev.Max - prev.Min) {
				continue
			}
		}
		out[idx] = s
	}
	return out
}

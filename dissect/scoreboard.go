package dissect

import (
	"encoding/binary"

	"github.com/rs/zerolog/log"
)

type Scoreboard struct {
	Players []ScoreboardPlayer
}

type ScoreboardPlayer struct {
	ID               []byte
	Score            uint32
	Assists          uint32
	AssistsFromRound uint32
}

// this function fixes kills that were previously recorded as elims
func readScoreboardKills(r *Reader) error {
	kills, err := r.Uint32()
	if err != nil {
		return err
	}
	if err := r.Skip(30); err != nil {
		return err
	}
	id, err := r.Bytes(4)
	if err != nil {
		return err
	}
	idx := r.PlayerIndexByID(id)
	if idx != -1 {
		username := r.Header.Players[idx].Username
		r.lastKillerFromScoreboard = username
		log.Warn().
			Str("username", username).
			Uint32("kills", kills).
			Msg("scoreboard_kill")
	}
	return nil
}

// is23PrefixRecord reports whether the tag the listener just matched on
// sits inside the Y11S1+ per-entity property envelope, i.e.:
//
//	23 <4B entity> 00 00 00 00 <4B tag>
//
// Returns the entity ref and true when the envelope checks out. The
// tag length `tagLen` is the tag byte-count (4 for scoreboard tags).
func is23PrefixRecord(r *Reader, tagLen int) (uint32, bool) {
	pre := tagLen + 9 // 9 = 1 (0x23) + 4 (entity) + 4 (zero pad)
	if r.offset < pre {
		return 0, false
	}
	if r.b[r.offset-pre] != 0x23 {
		return 0, false
	}
	for i := r.offset - 4 - tagLen; i < r.offset-tagLen; i++ {
		if r.b[i] != 0 {
			return 0, false
		}
	}
	entStart := r.offset - 8 - tagLen
	return binary.LittleEndian.Uint32(r.b[entStart : entStart+4]), true
}

func readScoreboardAssists(r *Reader) error {
	// Y11S1+ path — per-entity 23-prefix update. Entity-to-player
	// mapping is resolved in stats.go via playerForEntity.
	if ent, ok := is23PrefixRecord(r, 4); ok {
		if r.offset+5 > len(r.b) || r.b[r.offset] != 0x04 {
			return nil
		}
		assists := binary.LittleEndian.Uint32(r.b[r.offset+1 : r.offset+5])
		r.offset += 5
		if r.scoreboardAssists == nil {
			r.scoreboardAssists = make(map[uint32]uint32)
		}
		if assists > r.scoreboardAssists[ent] {
			r.scoreboardAssists[ent] = assists
		}
		return nil
	}
	assists, err := r.Uint32()
	if err != nil {
		return err
	}
	if assists == 0 {
		return nil
	}
	if err = r.Skip(30); err != nil {
		return err
	}
	id, err := r.Bytes(4)
	if err != nil {
		return err
	}
	idx := r.PlayerIndexByID(id)
	username := "N/A"
	if idx != -1 {
		username = r.Header.Players[idx].Username
		r.Scoreboard.Players[idx].Assists = assists
		r.Scoreboard.Players[idx].AssistsFromRound++
	}
	log.Debug().
		Uint32("assists", assists).
		Str("username", username).
		Msg("scoreboard_assists")
	return nil
}

func readScoreboardScore(r *Reader) error {
	// Y11S1+ path — 23-prefix per-entity score update.
	if ent, ok := is23PrefixRecord(r, 4); ok {
		if r.offset+5 > len(r.b) || r.b[r.offset] != 0x04 {
			return nil
		}
		score := binary.LittleEndian.Uint32(r.b[r.offset+1 : r.offset+5])
		r.offset += 5
		if r.scoreboardScore == nil {
			r.scoreboardScore = make(map[uint32]uint32)
		}
		if score > r.scoreboardScore[ent] {
			r.scoreboardScore[ent] = score
		}
		return nil
	}
	score, err := r.Uint32()
	if err != nil {
		return err
	}
	if score == 0 {
		return nil
	}
	if err = r.Skip(13); err != nil {
		return err
	}
	id, err := r.Bytes(4)
	if err != nil {
		return err
	}
	idx := r.PlayerIndexByID(id)
	username := "N/A"
	if idx != -1 {
		username = r.Header.Players[idx].Username
		r.Scoreboard.Players[idx].Score = score
	}
	log.Debug().
		Uint32("score", score).
		Str("username", username).
		Msg("scoreboard_score")
	return nil
}

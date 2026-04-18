package dissect

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	"github.com/rs/zerolog/log"
)

type MatchUpdateType int

//go:generate stringer -type=MatchUpdateType
const (
	Kill MatchUpdateType = iota
	Death
	DefuserPlantStart
	DefuserPlantComplete
	DefuserDisableStart
	DefuserDisableComplete
	LocateObjective
	OperatorSwap
	Battleye
	PlayerLeave
	Other
)

type MatchUpdate struct {
	Type                   MatchUpdateType `json:"type"`
	Username               string          `json:"username,omitempty"`
	Target                 string          `json:"target,omitempty"`
	Headshot               *bool           `json:"headshot,omitempty"`
	Time                   string          `json:"time"`
	TimeInSeconds          float64         `json:"timeInSeconds"`
	Message                string          `json:"message,omitempty"`
	Operator               Operator        `json:"operator,omitempty"`
	// WeaponID is the 8-byte weapon identifier emitted by the kill packet.
	// Same value repeats across consecutive kills by the same player on the
	// same weapon; changes when they swap primary/secondary/melee. No name
	// mapping exists yet — expose the raw id so downstream tools can resolve
	// it once a weapon dictionary is built.
	WeaponID uint64 `json:"weaponID,omitempty"`
	// killOffset is the file offset at which the kill envelope started.
	// Used by stats.go to look up the victim's HP just before the
	// kill for precise damage-per-blow attribution. Not serialized.
	killOffset             int
	usernameFromScoreboard string
}

func (i MatchUpdateType) MarshalJSON() (text []byte, err error) {
	return json.Marshal(stringerIntMarshal{
		Name: i.String(),
		ID:   int(i),
	})
}

func (i *MatchUpdateType) UnmarshalJSON(data []byte) (err error) {
	var x stringerIntMarshal
	if err = json.Unmarshal(data, &x); err != nil {
		return
	}
	*i = MatchUpdateType(x.ID)
	return
}

var activity2 = []byte{0x00, 0x00, 0x00, 0x22, 0xe3, 0x09, 0x00, 0x79}
var killIndicator = []byte{0x22, 0xd9, 0x13, 0x3c, 0xba}

// killPacketWeaponMarker is the 5-byte tag prefixing the 8-byte weapon id
// that follows the kill packet's headshot byte. Verified constant across all
// Y8S1 / Y8S2 / Y9S1 test replays. If it stops matching we log and skip the
// extra decode rather than misreading the next packet.
var killPacketWeaponMarker = []byte{0x22, 0xF8, 0x6C, 0xDD, 0x65}

func readMatchFeedback(r *Reader) error {
	if r.Header.CodeVersion >= Y9S1Update3 {
		if err := r.Skip(38); err != nil {
			return err
		}
	} else if r.Header.CodeVersion >= Y9S1 {
		if err := r.Skip(9); err != nil {
			return err
		}
		valid, err := r.Int()
		if err != nil {
			return err
		}
		if valid != 4 {
			return errors.New("match feedback failed valid check")
		}
		if err := r.Skip(24); err != nil {
			return err
		}
	} else {
		if err := r.Skip(1); err != nil {
			return err
		}
		if err := r.Seek(activity2); err != nil {
			return err
		}
	}
	size, err := r.Int()
	if err != nil {
		return err
	}
	if size == 0 { // kill or an unknown indicator at start of match
		killTrace, err := r.Bytes(5)
		if err != nil {
			return err
		}
		if !bytes.Equal(killTrace, killIndicator) {
			log.Debug().Hex("killTrace", killTrace).Send()
			return nil
		}
		username, err := r.String()
		if err != nil {
			return err
		}
		empty := len(username) == 0
		if empty {
			log.Debug().Str("warn", "kill username empty").Send()
		}
		// The kill packet is a sequence of field-framed entries, each laid
		// out as [5-byte tag][1-byte size][size bytes]. Between killer and
		// target the framing takes 15 bytes:
		//   22 C1 98 DE 70  04  XX 00 00 00   — uint32: killer team index+1
		//   22 AC 19 0F 70                     — tag for the target-name string
		// Values are observed but not exposed; we just skip past them and
		// let r.String() below pick up the length-prefixed target username.
		if err = r.Skip(15); err != nil {
			return err
		}
		target, err := r.String()
		if err != nil {
			return err
		}
		if empty && len(target) > 0 {
			u := MatchUpdate{
				Type:          Death,
				Username:      target,
				Time:          r.timeRaw,
				TimeInSeconds: r.time,
			}
			r.MatchFeedback = append(r.MatchFeedback, u)
			log.Debug().Interface("match_update", u).Send()
			log.Debug().Msg("kill username empty because of death")
			return nil
		} else if empty {
			return nil
		}
		u := MatchUpdate{
			Type:          Kill,
			Username:      username,
			Target:        target,
			Time:          r.timeRaw,
			TimeInSeconds: r.time,
			killOffset:    r.offset,
		}
		// Between target-name and the headshot value, five 10-byte uint32
		// fields plus the 6-byte header of the headshot field:
		//   22 78 2E 7B 50 04 XX 00 00 00      — uint32: target team index+1
		//   22 8D A8 3D D1 04 XX 00 00 00      — uint32: killer team index+1 (repeat)
		//   22 53 B8 87 31 04 XX 00 00 00      — uint32: target team index+1 (repeat)
		//   22 A5 AD 64 0B 04 00 00 00 00      — uint32: always 0 (role? assist?)
		//   22 90 3E BF 37 04 01 00 00 00      — uint32: always 1 (kill confirmed?)
		//   22 C3 5B A4 4E 01                  — tag+size for the 1-byte headshot flag
		// Total: 5*10 + 6 = 56. The headshot value itself follows as an Int().
		if err = r.Skip(56); err != nil {
			return err
		}
		headshot, err := r.Int()
		if err != nil {
			return err
		}
		headshotPtr := new(bool)
		if headshot == 1 {
			*headshotPtr = true
		}
		u.Headshot = headshotPtr
		// Weapon id — a uint64 hash identifying the weapon used. Stable across
		// consecutive kills by the same player on the same weapon. We check
		// the tag before reading so that unknown packet variants (future
		// seasons, unseen kill subtypes) don't misalign the stream.
		mark, err := r.Bytes(5)
		if err != nil {
			return err
		}
		if bytes.Equal(mark, killPacketWeaponMarker) {
			wid, err := r.Uint64()
			if err != nil {
				return err
			}
			u.WeaponID = wid
		} else {
			log.Debug().Hex("got", mark).Msg("kill: weapon-id marker mismatch, skipping weapon decode")
		}
		// Ignore duplicates
		for _, val := range r.MatchFeedback {
			if val.Type == Kill && val.Username == u.Username && val.Target == u.Target {
				return nil
			}
		}
		// removing the elimination username for now
		if r.lastKillerFromScoreboard != username {
			u.usernameFromScoreboard = r.lastKillerFromScoreboard
		}
		r.MatchFeedback = append(r.MatchFeedback, u)
		log.Debug().Interface("match_update", u).Send()
		return nil
	}
	// TODO: Y9S1 may have removed or modified other match feedback options
	if r.Header.CodeVersion >= Y9S1 {
		return nil
	}
	b, err := r.Bytes(size)
	if err != nil {
		return err
	}
	msg := string(b)
	t := Other
	if strings.Contains(msg, "bombs") || strings.Contains(msg, "objective") {
		t = LocateObjective
	}
	if strings.Contains(msg, "BattlEye") {
		t = Battleye
	}
	if strings.Contains(msg, "left") {
		t = PlayerLeave
	}
	username := strings.Split(msg, " ")[0]
	if t == Other {
		username = ""
	} else {
		msg = ""
	}
	u := MatchUpdate{
		Type:          t,
		Username:      username,
		Target:        "",
		Time:          r.timeRaw,
		TimeInSeconds: r.time,
		Message:       msg,
	}
	r.MatchFeedback = append(r.MatchFeedback, u)
	log.Debug().Interface("match_update", u).Send()
	return nil
}

package dissect

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
)

func readTime(r *Reader) error {
	time, err := r.Uint32()
	if err != nil {
		return err
	}
	r.time = float64(time)
	r.timeRaw = fmt.Sprintf("%d:%02d", time/60, time%60)
	return nil
}

func readY7Time(r *Reader) error {
	time, err := r.String()
	parts := strings.Split(time, ":")
	if len(parts) == 1 {
		seconds, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return err
		}
		r.time = seconds
		r.timeRaw = parts[0]
		return nil
	}
	minutes, err := strconv.Atoi(parts[0])
	if err != nil {
		return err
	}
	seconds, err := strconv.Atoi(parts[1])
	if err != nil {
		return err
	}
	r.time = float64((minutes * 60) + seconds)
	r.timeRaw = time
	return nil
}

func (r *Reader) roundEnd() {
	log.Debug().Msg("round_end")

	planter := -1
	deaths := make(map[int]int)
	sizes := make(map[int]int)
	roles := make(map[int]TeamRole)

	for _, p := range r.Header.Players {
		sizes[p.TeamIndex] += 1
		roles[p.TeamIndex] = r.Header.Teams[p.TeamIndex].Role
	}

	if r.Header.CodeVersion >= Y9S4 {
		team0Won := r.Header.Teams[0].StartingScore < r.Header.Teams[0].Score
		r.Header.Teams[0].Won = team0Won
		r.Header.Teams[1].Won = !team0Won
	}

	for _, u := range r.MatchFeedback {
		switch u.Type {
		case Kill:
			i := r.Header.Players[r.PlayerIndexByUsername(u.Target)].TeamIndex
			deaths[i] = deaths[i] + 1
			// fix killer username
			if len(u.usernameFromScoreboard) > 0 {
				u.Username = u.usernameFromScoreboard
			}
			break
		case Death:
			i := r.Header.Players[r.PlayerIndexByUsername(u.Username)].TeamIndex
			deaths[i] = deaths[i] + 1
			break
		case DefuserPlantComplete:
			planter = r.PlayerIndexByUsername(u.Username)
			break
		case DefuserDisableComplete:
			disablerTeam := r.Header.Players[r.PlayerIndexByUsername(u.Username)].TeamIndex
			// On Y9S4+, Won is already authoritative from StartingScore -> Score
			// (set above). Only attach the WinCondition, and only if the event
			// is role-consistent (defenders disable; attacker-attributed disable
			// events are misclassifications we shouldn't trust).
			if r.Header.CodeVersion >= Y9S4 {
				if r.Header.Teams[disablerTeam].Won && roles[disablerTeam] == Defense {
					r.Header.Teams[disablerTeam].WinCondition = DisabledDefuser
				}
				return
			}
			r.Header.Teams[disablerTeam].Won = true
			r.Header.Teams[disablerTeam].WinCondition = DisabledDefuser
			return
		}
	}

	if planter > -1 {
		planterTeam := r.Header.Players[planter].TeamIndex
		// Same guard as DefuserDisableComplete: Y9S4+ trusts StartingScore for
		// Won, and only attackers can plant. Defender-attributed plant events
		// (caused by event-type misclassification) would otherwise mark both
		// teams as winners and mis-label the WinCondition on the losing side.
		if r.Header.CodeVersion >= Y9S4 {
			if r.Header.Teams[planterTeam].Won && roles[planterTeam] == Attack {
				r.Header.Teams[planterTeam].WinCondition = DefusedBomb
				return
			}
			// Fall through to the Y9S4 heuristic fallback — the planter event
			// was attributed to someone who can't actually plant, so we can't
			// trust the attribution but we can still try to derive a sensible
			// WinCondition from the overall round shape.
		} else {
			r.Header.Teams[planterTeam].Won = true
			r.Header.Teams[planterTeam].WinCondition = DefusedBomb
			return
		}
	}

	// Y9S4+ WinCondition fallback. Won is already authoritatively set from
	// StartingScore -> Score above, so we only derive a WinCondition when the
	// matchFeedback loop couldn't attach one (typically because defuser events
	// were misclassified by the reader). The heuristic is deliberately
	// conservative: prefer known round-shape signals; leave blank if unsure.
	if r.Header.CodeVersion >= Y9S4 {
		winner := 0
		if r.Header.Teams[1].Won {
			winner = 1
		}
		if r.Header.Teams[winner].WinCondition == "" {
			loser := 1 - winner
			sawPlant := false
			sawDisable := false
			for _, u := range r.MatchFeedback {
				if u.Type == DefuserPlantComplete {
					sawPlant = true
				} else if u.Type == DefuserDisableComplete {
					sawDisable = true
				}
			}
			switch {
			case sawPlant && roles[winner] == Attack:
				// Attackers won AND a plant event fired — almost certainly
				// they defused (regardless of which player the event was
				// attributed to).
				r.Header.Teams[winner].WinCondition = DefusedBomb
			case sawDisable && roles[winner] == Defense:
				r.Header.Teams[winner].WinCondition = DisabledDefuser
			case sizes[loser] > 0 && deaths[loser] == sizes[loser]:
				r.Header.Teams[winner].WinCondition = KilledOpponents
			case roles[winner] == Defense:
				// Defenders win-by-time is the last-resort explanation if no
				// defuser events and not all attackers died.
				r.Header.Teams[winner].WinCondition = Time
			}
		}
		return
	}

	if deaths[0] == sizes[0] {
		if planter > -1 && roles[0] == Attack { // ignore attackers killed post-plant
			return
		}
		r.Header.Teams[1].Won = true
		r.Header.Teams[1].WinCondition = KilledOpponents
		return
	}
	if deaths[1] == sizes[1] {
		if planter > -1 && roles[1] == Attack { // ignore attackers killed post-plant
			return
		}
		r.Header.Teams[0].Won = true
		r.Header.Teams[0].WinCondition = KilledOpponents
		return
	}

	i := 0
	if roles[1] == Defense {
		i = 1
	}

	r.Header.Teams[i].Won = true
	r.Header.Teams[i].WinCondition = Time
}

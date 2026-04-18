package dissect

import "sort"

// ClutchBreakdown splits clutch wins by size (1v1, 1v2, …). Mirrors the
// scoreline coaches typically ask for instead of a single opaque count.
type ClutchBreakdown struct {
	OneV1 int `json:"1v1,omitempty"`
	OneV2 int `json:"1v2,omitempty"`
	OneV3 int `json:"1v3,omitempty"`
	OneV4 int `json:"1v4,omitempty"`
	OneV5 int `json:"1v5,omitempty"`
}

func (c *ClutchBreakdown) add(size int) {
	switch size {
	case 1:
		c.OneV1++
	case 2:
		c.OneV2++
	case 3:
		c.OneV3++
	case 4:
		c.OneV4++
	case 5:
		c.OneV5++
	}
}

// WeaponStat aggregates kills by weapon id over a match. WeaponID is the
// raw 8-byte identifier from the kill packet — a name dictionary isn't
// shipped yet, so downstream tooling resolves it.
type WeaponStat struct {
	WeaponID  uint64 `json:"weaponID"`
	Kills     int    `json:"kills"`
	Headshots int    `json:"headshots"`
}

// actionPhaseSeconds returns the expected action-phase duration for the
// current game mode. Used to convert the kill event's count-down
// TimeInSeconds into an "elapsed seconds alive" metric. Defaults to
// Bomb's 180s for unknown modes.
func (r *Reader) actionPhaseSeconds() int {
	switch r.Header.GameMode {
	case SecureArea:
		return 240
	default:
		return 180
	}
}

type PlayerRoundStats struct {
	Username           string  `json:"username"`
	TeamIndex          int     `json:"-"`
	Score              int     `json:"score"`
	Operator           string  `json:"-"`
	Kills              int     `json:"kills"`
	Died               bool    `json:"died"`
	Assists            int     `json:"assists"`
	Headshots          int     `json:"headshots"`
	HeadshotPercentage float64 `json:"headshotPercentage"`
	// DamageTaken is derived from the HP stream: startHP - minHP observed
	// over the round. Omitted when no HP samples were captured (older
	// replay versions or non-participating slots).
	DamageTaken int `json:"damageTaken,omitempty"`
	// DamageDealt is an approximation: for each kill by this player, the
	// victim's starting HP (or 100 if unknown) is credited. Misses
	// assist damage and damage on round survivors — treat as a floor.
	DamageDealt int `json:"damageDealt,omitempty"`
	OneVx       int `json:"1vX,omitempty"`
	// DeathTime mirrors the kill event's m:ss clock (count-down format)
	// for this player's death. Empty when they survived the round.
	DeathTime string `json:"deathTime,omitempty"`
	// SecondsAlive is elapsed action-phase seconds. Full round duration
	// for survivors, actionPhaseSeconds - deathTimeInSeconds otherwise.
	SecondsAlive int `json:"secondsAlive,omitempty"`
	// EntryKill / EntryDeath mark the killer and victim of the round's
	// first kill (what CS/Valorant call "opening duel").
	EntryKill  bool `json:"entryKill,omitempty"`
	EntryDeath bool `json:"entryDeath,omitempty"`
	// OperatorSwaps counts mid-prep/mid-round operator changes for
	// this player. High swap counts are an indecision signal.
	OperatorSwaps int `json:"operatorSwaps,omitempty"`
	// Trade bookkeeping — a death is "traded" when a teammate kills
	// the killer within 3s; a "trade kill" is the avenging kill.
	// Untraded deaths are uncontested — coaches flag these as bad
	// positioning or isolation.
	TradedDeath   bool `json:"tradedDeath,omitempty"`
	UntradedDeath bool `json:"untradedDeath,omitempty"`
	TradeKill     bool `json:"tradeKill,omitempty"`
	// PlantedDefuser is true for the attacker who completed the
	// plant; DisabledDefuser for the defender who completed the
	// disable. At most one of each per round in a normal Bomb match.
	PlantedDefuser  bool `json:"plantedDefuser,omitempty"`
	DisabledDefuser bool `json:"disabledDefuser,omitempty"`
}

type PlayerMatchStats struct {
	Username           string  `json:"username"`
	TeamIndex          int     `json:"-"`
	Rounds             int     `json:"rounds"`
	Kills              int     `json:"kills"`
	Deaths             int     `json:"deaths"`
	Assists            int     `json:"assists"`
	Score              int     `json:"score"`
	Headshots          int     `json:"headshots"`
	HeadshotPercentage float64 `json:"headshotPercentage"`
	DamageTaken        int     `json:"damageTaken,omitempty"`
	DamageDealt        int     `json:"damageDealt,omitempty"`
	EntryKills         int     `json:"entryKills,omitempty"`
	EntryDeaths        int     `json:"entryDeaths,omitempty"`
	OperatorSwaps      int     `json:"operatorSwaps,omitempty"`
	TradedDeaths       int     `json:"tradedDeaths,omitempty"`
	UntradedDeaths     int     `json:"untradedDeaths,omitempty"`
	TradeKills         int     `json:"tradeKills,omitempty"`
	DefuserPlants      int     `json:"defuserPlants,omitempty"`
	DefuserDisables    int     `json:"defuserDisables,omitempty"`
	// AvgSecondsAlive is the per-round mean of SecondsAlive across all
	// rounds played — survivors contribute the full round duration.
	AvgSecondsAlive float64         `json:"avgSecondsAlive,omitempty"`
	Clutches        ClutchBreakdown `json:"clutches,omitempty"`
	// Weapons lists per-weapon kill/headshot counts across the match,
	// sorted by kill count descending.
	Weapons []WeaponStat `json:"weapons,omitempty"`
}

// OpeningKill returns the first player to kill.
func (r *Reader) OpeningKill() MatchUpdate {
	for _, a := range r.MatchFeedback {
		if a.Type == Kill {
			return a
		}
	}
	return MatchUpdate{}
}

// OpeningDeath returns the first player to die (KILL or DEATH activity).
func (r *Reader) OpeningDeath() MatchUpdate {
	for _, a := range r.MatchFeedback {
		if a.Type == Kill || a.Type == Death {
			return a
		}
	}
	return MatchUpdate{}
}

// Trades returns KILL Activity pairs of trades.
func (r *Reader) Trades() [][]MatchUpdate {
	trades := make([][]MatchUpdate, 0)
	var previous = MatchUpdate{}
	for _, a := range r.MatchFeedback {
		var samePlayers = (previous.Target == a.Username || previous.Username == a.Target)
		var withinThreshold = (previous.TimeInSeconds - a.TimeInSeconds) <= 3
		if a.Type == Kill && samePlayers && withinThreshold {
			trades = append(trades, []MatchUpdate{previous, a})
		}
		previous = a
	}
	return trades
}

func (r *Reader) KillsAndDeaths() []MatchUpdate {
	MatchFeedback := make([]MatchUpdate, 0)
	for _, a := range r.MatchFeedback {
		if a.Type == Kill || a.Type == Death {
			MatchFeedback = append(MatchFeedback, a)
		}
	}
	return MatchFeedback
}

func (r *Reader) NumPlayers(team int) int {
	n := 0
	for _, p := range r.Header.Players {
		if p.TeamIndex == team {
			n++
		}
	}
	return n
}

func (r *Reader) PlayerStats() []PlayerRoundStats {
	stats := make([]PlayerRoundStats, 0)
	index := make(map[string]int)
	winningTeamIndex := 0
	if r.Header.Teams[1].Won {
		winningTeamIndex = 1
	}
	// Resolve 23-prefix entity maps to player index once.
	scoreByPlayer := make(map[int]uint32)
	for ent, v := range r.scoreboardScore {
		if idx, _ := r.playerForEntity(ent); idx >= 0 && v > scoreByPlayer[idx] {
			scoreByPlayer[idx] = v
		}
	}
	assistsByPlayer := make(map[int]uint32)
	for ent, v := range r.scoreboardAssists {
		if idx, _ := r.playerForEntity(ent); idx >= 0 && v > assistsByPlayer[idx] {
			assistsByPlayer[idx] = v
		}
	}
	for i, p := range r.Header.Players {
		scorePlayer := r.Scoreboard.Players[i]
		score := int(scorePlayer.Score)
		if s, ok := scoreByPlayer[i]; ok && int(s) > score {
			score = int(s)
		}
		assists := int(scorePlayer.AssistsFromRound)
		if a, ok := assistsByPlayer[i]; ok && int(a) > assists {
			assists = int(a)
		}
		stats = append(stats, PlayerRoundStats{
			Username:  p.Username,
			TeamIndex: p.TeamIndex,
			Operator:  p.Operator.String(),
			Assists:   assists,
			Score:     score,
		})
		index[p.Username] = i
	}
	healthByPlayer := r.healthByPlayer()
	actionSec := r.actionPhaseSeconds()
	firstKillSeen := false
	lastDeath := -1
	for idx, a := range r.MatchFeedback {
		i, ok := index[a.Username]
		if !ok {
			i = -1
		}
		switch a.Type {
		case Kill:
			ti, tok := index[a.Target]
			if i >= 0 {
				stats[i].Kills += 1
				if a.Headshot != nil && *a.Headshot {
					stats[i].Headshots += 1
				}
				stats[i].HeadshotPercentage = headshotPercentage(stats[i].Headshots, stats[i].Kills)
			}
			if tok {
				stats[ti].Died = true
				stats[ti].DeathTime = a.Time
				stats[ti].SecondsAlive = clampNonNeg(actionSec - int(a.TimeInSeconds))
				lastDeath = ti
				// Damage-dealt floor: credit killer with the victim's
				// observed starting HP, 100 if unknown.
				dmg := 100
				if s, ok := healthByPlayer[ti]; ok && s.Max > 0 {
					dmg = int(s.Max)
				}
				if i >= 0 {
					stats[i].DamageDealt += dmg
				}
			}
			// Mark the round's very first kill as entry kill/death.
			if !firstKillSeen {
				firstKillSeen = true
				if i >= 0 {
					stats[i].EntryKill = true
				}
				if tok {
					stats[ti].EntryDeath = true
				}
			}
			// Trade-kill bookkeeping: a kill is "traded" when the
			// killer dies within 3s. The victim's death is then
			// TradedDeath; the avenging killer gets TradeKill. If no
			// revenge kill lands inside the window, the victim's
			// death is UntradedDeath.
			if i >= 0 && tok {
				traded := false
				for j := idx + 1; j < len(r.MatchFeedback); j++ {
					b := r.MatchFeedback[j]
					if (a.TimeInSeconds - b.TimeInSeconds) > 3 {
						break
					}
					if b.Type != Kill || b.Target != a.Username {
						continue
					}
					traded = true
					if bi, bok := index[b.Username]; bok {
						stats[bi].TradeKill = true
					}
					break
				}
				if traded {
					stats[ti].TradedDeath = true
				} else {
					stats[ti].UntradedDeath = true
				}
			}
		case Death:
			if i >= 0 {
				stats[i].Died = true
				stats[i].DeathTime = a.Time
				stats[i].SecondsAlive = clampNonNeg(actionSec - int(a.TimeInSeconds))
				stats[i].UntradedDeath = true
				lastDeath = i
			}
		case OperatorSwap:
			if i >= 0 {
				stats[i].OperatorSwaps++
			}
		case DefuserPlantComplete:
			if i >= 0 {
				stats[i].PlantedDefuser = true
			}
		case DefuserDisableComplete:
			if i >= 0 {
				stats[i].DisabledDefuser = true
			}
		}
	}
	// Survivors spent the full action phase alive.
	for i := range stats {
		if !stats[i].Died {
			stats[i].SecondsAlive = actionSec
		}
	}
	// Fill DamageTaken from the HP min/max window per player. Players
	// who died always took their full starting HP — the HP stream
	// frequently stops updating before the killing blow (especially on
	// one-shot headshots), so we fall back to Max for the died case.
	for i := range stats {
		s, ok := healthByPlayer[i]
		if !ok {
			continue
		}
		if stats[i].Died {
			stats[i].DamageTaken = int(s.Max)
		} else {
			stats[i].DamageTaken = int(s.Max - s.Min)
		}
	}
	// Calculates 1vX
	winnersLeftAlive := make([]int, 0)
	lastDeathWasWinner := false
	for i, p := range r.Header.Players {
		if p.TeamIndex != winningTeamIndex {
			continue
		}
		if !stats[i].Died {
			winnersLeftAlive = append(winnersLeftAlive, i)
		}
		if i == lastDeath {
			lastDeathWasWinner = true
		}
	}
	nWinnersLeftAlive := len(winnersLeftAlive)
	lastWinnerStanding := -1
	if nWinnersLeftAlive == 1 {
		lastWinnerStanding = winnersLeftAlive[0]
	} else if nWinnersLeftAlive == 0 && lastDeathWasWinner {
		lastWinnerStanding = lastDeath
	}
	if lastWinnerStanding > -1 {
		username := stats[lastWinnerStanding].Username
		teamLeft := r.NumPlayers(winningTeamIndex)
		oneVx := 0
		for _, a := range r.MatchFeedback {
			if a.Type == Kill && stats[index[a.Target]].TeamIndex == winningTeamIndex {
				teamLeft--
			} else if a.Type == Death && stats[index[a.Username]].TeamIndex == winningTeamIndex {
				teamLeft--
			} else if a.Type == PlayerLeave && stats[index[a.Username]].TeamIndex == winningTeamIndex {
				teamLeft--
			}
			if a.Username != username {
				continue
			}
			if a.Type == Kill && teamLeft < 2 {
				oneVx++
			}
		}
		for _, s := range stats {
			if s.TeamIndex != winningTeamIndex && !s.Died {
				oneVx++
			}
		}
		stats[lastWinnerStanding].OneVx = oneVx
	}
	return stats
}

func (m *MatchReader) PlayerStats() []PlayerMatchStats {
	stats := make([]PlayerMatchStats, 0)
	index := make(map[string]int)
	// Per-player weapon tallies: [userIdx][weaponID] -> WeaponStat.
	weapons := make([]map[uint64]*WeaponStat, 0)
	// Running sum of SecondsAlive per player for AvgSecondsAlive.
	secondsAliveTotal := make([]int, 0)
	for _, r := range m.rounds {
		roundStats := r.PlayerStats()
		for _, p := range roundStats {
			if len(stats) == 0 || stats[index[p.Username]].Username != p.Username {
				stats = append(stats, PlayerMatchStats{
					Username:  p.Username,
					TeamIndex: p.TeamIndex,
				})
				index[p.Username] = len(index)
				weapons = append(weapons, make(map[uint64]*WeaponStat))
				secondsAliveTotal = append(secondsAliveTotal, 0)
			}
			i := index[p.Username]
			stats[i].Rounds += 1
			stats[i].Kills += p.Kills
			if p.Died {
				stats[i].Deaths += 1
			}
			stats[i].Assists += p.Assists
			stats[i].Score += p.Score
			stats[i].Headshots += p.Headshots
			stats[i].HeadshotPercentage = headshotPercentage(stats[i].Headshots, stats[i].Kills)
			stats[i].DamageDealt += p.DamageDealt
			stats[i].DamageTaken += p.DamageTaken
			if p.EntryKill {
				stats[i].EntryKills++
			}
			if p.EntryDeath {
				stats[i].EntryDeaths++
			}
			stats[i].OperatorSwaps += p.OperatorSwaps
			if p.TradedDeath {
				stats[i].TradedDeaths++
			}
			if p.UntradedDeath {
				stats[i].UntradedDeaths++
			}
			if p.TradeKill {
				stats[i].TradeKills++
			}
			if p.PlantedDefuser {
				stats[i].DefuserPlants++
			}
			if p.DisabledDefuser {
				stats[i].DefuserDisables++
			}
			secondsAliveTotal[i] += p.SecondsAlive
			// Clutch breakdown — OneVx carries the size (1..5) of the
			// clutch this player won for their team that round.
			if p.OneVx > 0 {
				stats[i].Clutches.add(p.OneVx)
			}
		}
		// Weapon aggregation needs raw MatchFeedback (round stats
		// drops per-kill data once aggregated).
		for _, a := range r.MatchFeedback {
			if a.Type != Kill || a.WeaponID == 0 {
				continue
			}
			i, ok := index[a.Username]
			if !ok {
				continue
			}
			ws := weapons[i][a.WeaponID]
			if ws == nil {
				ws = &WeaponStat{WeaponID: a.WeaponID}
				weapons[i][a.WeaponID] = ws
			}
			ws.Kills++
			if a.Headshot != nil && *a.Headshot {
				ws.Headshots++
			}
		}
	}
	// Materialize weapon maps as sorted slices and compute averages.
	for i := range stats {
		ws := make([]WeaponStat, 0, len(weapons[i]))
		for _, s := range weapons[i] {
			ws = append(ws, *s)
		}
		sort.Slice(ws, func(a, b int) bool {
			if ws[a].Kills != ws[b].Kills {
				return ws[a].Kills > ws[b].Kills
			}
			return ws[a].WeaponID < ws[b].WeaponID
		})
		stats[i].Weapons = ws
		if stats[i].Rounds > 0 {
			stats[i].AvgSecondsAlive = float64(secondsAliveTotal[i]) / float64(stats[i].Rounds)
		}
	}
	return stats
}

func headshotPercentage(headshots, kills int) float64 {
	if kills == 0 {
		return 0
	}
	return float64(headshots) / float64(kills) * 100
}

func clampNonNeg(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

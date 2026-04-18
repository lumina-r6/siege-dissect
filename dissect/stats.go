package dissect

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
}

type PlayerMatchStats struct {
	Username           string  `json:"username"`
	TeamIndex          int     `json:"-"`
	Rounds             int     `json:"rounds"`
	Kills              int     `json:"kills"`
	Deaths             int     `json:"deaths"`
	Assists            int     `json:"assists"`
	Headshots          int     `json:"headshots"`
	HeadshotPercentage float64 `json:"headshotPercentage"`
	DamageTaken        int     `json:"damageTaken,omitempty"`
	DamageDealt        int     `json:"damageDealt,omitempty"`
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
	for i, p := range r.Header.Players {
		scorePlayer := r.Scoreboard.Players[i]
		stats = append(stats, PlayerRoundStats{
			Username:  p.Username,
			TeamIndex: p.TeamIndex,
			Operator:  p.Operator.String(),
			Assists:   int(scorePlayer.AssistsFromRound),
			Score:     int(scorePlayer.Score),
		})
		index[p.Username] = i
	}
	healthByPlayer := r.healthByPlayer()
	lastDeath := -1
	for _, a := range r.MatchFeedback {
		i := index[a.Username]
		if a.Type == Kill {
			stats[i].Kills += 1
			if *a.Headshot {
				stats[i].Headshots += 1
			}
			stats[i].HeadshotPercentage = headshotPercentage(stats[i].Headshots, stats[i].Kills)
			ti := index[a.Target]
			stats[ti].Died = true
			lastDeath = ti
			// Credit the killer with the victim's starting HP as a
			// damage-dealt floor. Falls back to 100 if HP was never
			// observed for the victim (e.g. they never took damage
			// before dying, or HP stream wasn't captured).
			dmg := 100
			if s, ok := healthByPlayer[ti]; ok && s.Max > 0 {
				dmg = int(s.Max)
			}
			stats[i].DamageDealt += dmg
		} else if a.Type == Death {
			stats[i].Died = true
			lastDeath = i
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
	for i, r := range m.rounds {
		for _, p := range r.PlayerStats() {
			if len(stats) == 0 || stats[index[p.Username]].Username != p.Username {
				stats = append(stats, PlayerMatchStats{
					Username:  p.Username,
					TeamIndex: p.TeamIndex,
				})
				index[p.Username] = len(index)
			}
			i = index[p.Username]
			stats[i].Rounds += 1
			stats[i].Kills += p.Kills
			if p.Died {
				stats[i].Deaths += 1
			}
			stats[i].Assists += p.Assists
			stats[i].Headshots += p.Headshots
			stats[i].HeadshotPercentage = headshotPercentage(stats[i].Headshots, stats[i].Kills)
			stats[i].DamageDealt += p.DamageDealt
			stats[i].DamageTaken += p.DamageTaken
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

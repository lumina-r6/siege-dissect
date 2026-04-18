# siege-dissect
[![Go Reference](https://pkg.go.dev/badge/github.com/lumina-r6/siege-dissect.svg)](https://pkg.go.dev/github.com/lumina-r6/siege-dissect)

Match Replay API/CLI for Rainbow Six: Siege's Dissect (.rec) format.

A maintained fork of [redraskal/r6-dissect](https://github.com/redraskal/r6-dissect),
extended for coach-facing analysis work in the [Lumina R6](https://github.com/lumina-r6)
toolchain. Upstream credit and original design belong to Benjamin Ryan and
all prior contributors; this fork exists to land season-compatibility and
correctness fixes that the original repository hasn't absorbed.

**This is a work in progress. The data format is subject to change until a stable version is released.**

Download the latest version here: https://github.com/lumina-r6/siege-dissect/releases

## Current Features
- Match Info (Game version, map, gamemode, match type, teams, players)
- Match Feedback (Kills, headshots, objective locates, defuser plants/disables, BattlEye bans, DCs)
- JSON or Excel output

## Planned Features
- UI alternative
- Track bullet hits/misses
- Track movement packets
- Track other player statistics

## CLI Usage
Print a match overview by specifying a match folder or .rec file:
```bash
siege-dissect --info Match-2023-03-13_23-23-58-199
# or
siege-dissect --info Match-2023-03-13_23-23-58-199-R01.rec
```
```
5:20PM INF Version:          Y8S1/7422506
5:20PM INF Recording Player: redraskal [1f63af29-7ebe-48e7-b570-e820632d9565]
5:20PM INF Match ID:         d74d2685-193f-4fee-831f-41f8c7792250
5:20PM INF Timestamp:        2023-03-13 13:00:08 -0500 CDT
5:20PM INF Match Type:       QuickMatch
5:20PM INF Game Mode:        Bomb
5:20PM INF Map:              House
```
You can export round stats to a JSON file:
```bash
siege-dissect Match-2023-03-13_23-23-58-199-R01.rec -o round.json
```
Example:
```json
{
  "gameVersion": "Y8S1",
  "codeVersion": 7422506,
  "timestamp": "2023-03-13T23:25:46Z",
  "matchType": {
    "name": "Ranked",
    "id": 2
  },
  "map": {
    "name": "Villa",
    "id": 88107330328
  },
  "site": "2F Aviator Room, 2F Games Room",
  "recordingPlayerID": 15451868541914624436,
  "recordingProfileID": "1f63af29-7ebe-48e7-b570-e820632d9565",
  "additionalTags": "423855620",
  "gamemode": {
    "name": "Bomb",
    "id": 327933806
  },
...
  "teams": [
    {
      "name": "YOUR TEAM",
      "score": 1,
      "won": true,
      "winCondition": "KilledOpponents",
      "role": "Attack"
    },
    {
      "name": "OPPONENTS",
      "score": 0,
      "won": false,
      "role": "Defense"
    }
  ],
  "players": [
    {
      "id": 1830934665040226621,
      "profileID": "f33396d4-714b-442d-b110-9237e291cc71",
      "username": "IanFiftyForty",
      "teamIndex": 1,
      "operator": {
        "name": "Oryx",
        "id": 104189664155
      },
      "heroName": 243632506966,
      "alliance": 0,
      "roleImage": 104189664090,
      "roleName": "ORYX",
      "rolePortrait": 258649622576
    },
...
  "matchFeedback": [
    {
      "type": "Other",
      "time": "2:59",
      "timeInSeconds": 179,
      "message": "Friendly Fire is now active"
    },
    {
      "type": "Kill",
      "username": "ReithYT",
      "target": "Ambatakum.",
      "headshot": false,
      "time": "1:51",
      "timeInSeconds": 111
    },
...
```
Or the entire match:
```bash
siege-dissect Match-2023-03-13_23-23-58-199 -o match.json
```
Export an Excel spreadsheet by swapping .json with .xlsx.
```bash
siege-dissect Match-2023-03-13_23-23-58-199-R01 -o match.xlsx
```
Output JSON to the console (stdout) with the following syntax:
```bash
# entire match
siege-dissect Match-2023-03-13_23-23-58-199-R01
# or single round
siege-dissect Match-2023-03-13_23-23-58-199-R01/Match-2023-03-13_23-23-58-199-R01.rec
```

See example outputs in [/examples](https://github.com/lumina-r6/siege-dissect/tree/main/examples).

## Importing a .rec file
```go
package main

import (
	"log"
	"os"

	"github.com/lumina-r6/siege-dissect/dissect"
)

func main() {
	f, err := os.Open("Match-2023-03-13_23-23-58-199-R01.rec")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	r, err := dissect.NewReader(f)
	if err != nil {
		log.Fatal(err)
	}
	// Use r.ReadPartial() for faster reads with less data (designed to fill in data gaps in the header)
	// dissect.Ok(err) returns true if the error only pertains to EOF (read was successful)
	if err := r.Read(); !dissect.Ok(err) {
		log.Fatal(err)
	}
	print(r.Header.GameVersion) // Y8S1
}
```

## Credits

This project is a fork of [redraskal/r6-dissect](https://github.com/redraskal/r6-dissect) by Benjamin Ryan, originally MIT-licensed in 2022. Core reverse engineering credit additionally goes to [stnokott](https://github.com/stnokott) for upstream work on r6-dissect, and to [draguve](https://github.com/draguve) and other contributors at [draguve/R6-Replays](https://github.com/draguve/R6-Replays).

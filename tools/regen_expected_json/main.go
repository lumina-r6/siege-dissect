// regen_expected_json walks the test data directory, parses each .rec file
// with the current dissect library, and rewrites its sibling .rec.json with
// the Reader-struct-shaped output the tests expect.
//
// Used after changes that legitimately alter MatchFeedback (e.g. the Lane B
// dual-pattern op-swap fix). Run from repo root:
//
//	go run ./tools/regen_expected_json
//
// It only overwrites JSONs in dissect/test/data/replays/valid/ — invalid
// replay tests don't have expected JSONs.
package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/lumina-r6/siege-dissect/dissect"
)

const root = "dissect/test/data/replays/valid"

func main() {
	log.SetFlags(0)
	count := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".rec") {
			return nil
		}
		if err := regenOne(path); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		count++
		log.Printf("regenerated: %s.json", path)
		return nil
	})
	if err != nil {
		log.Fatalf("walk failed: %v", err)
	}
	log.Printf("done — %d files rewritten", count)
}

func regenOne(recPath string) error {
	f, err := os.Open(recPath)
	if err != nil {
		return err
	}
	defer f.Close()

	r, err := dissect.NewReader(f)
	if err != nil {
		return fmt.Errorf("NewReader: %w", err)
	}
	if err := r.Read(); !dissect.Ok(err) {
		return fmt.Errorf("Read: %w", err)
	}

	out, err := os.Create(recPath + ".json")
	if err != nil {
		return err
	}
	defer out.Close()

	enc := json.NewEncoder(out)
	enc.SetIndent("", "\t")
	return enc.Encode(r)
}

package vault

import (
	"crypto/sha256"
	"regexp"
	"sort"
	"strings"
)

type BrokenLink struct{ Source, Target string }
type DuplicateGroup struct{ Paths []string }
type IntegrityReport struct {
	OK          bool
	BrokenLinks []BrokenLink
	Duplicates  []DuplicateGroup
	Notes       int
}

var wikiLink = regexp.MustCompile(`\[\[([^\]|#]+)(?:#[^\]|]+)?(?:\|[^\]]+)?\]\]`)

func (v *Vault) CheckIntegrity() (IntegrityReport, error) {
	notes, err := v.ListAllNotes()
	if err != nil {
		return IntegrityReport{}, err
	}
	report := IntegrityReport{OK: true, Notes: len(notes)}
	known := map[string]bool{}
	hashes := map[[32]byte][]string{}
	for _, note := range notes {
		known[note.Path] = true
		hash := sha256.Sum256([]byte(strings.TrimSpace(note.Content)))
		hashes[hash] = append(hashes[hash], note.Path)
	}
	for _, note := range notes {
		for _, match := range wikiLink.FindAllStringSubmatch(note.Content, -1) {
			target := strings.TrimSpace(match[1])
			if !known[target] {
				report.BrokenLinks = append(report.BrokenLinks, BrokenLink{Source: note.Path, Target: target})
			}
		}
	}
	for _, paths := range hashes {
		if len(paths) > 1 {
			sort.Strings(paths)
			report.Duplicates = append(report.Duplicates, DuplicateGroup{Paths: paths})
		}
	}
	sort.Slice(report.BrokenLinks, func(i, j int) bool { return report.BrokenLinks[i].Source < report.BrokenLinks[j].Source })
	sort.Slice(report.Duplicates, func(i, j int) bool { return report.Duplicates[i].Paths[0] < report.Duplicates[j].Paths[0] })
	report.OK = len(report.BrokenLinks) == 0 && len(report.Duplicates) == 0
	return report, nil
}

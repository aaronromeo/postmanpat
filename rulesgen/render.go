package rulesgen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func writeFragment(dir, name string, data []byte) error {
	final := filepath.Join(dir, name)
	tmp, err := os.CreateTemp(dir, ".frag-*")
	if err != nil {
		return fmt.Errorf("rulesgen render: create temp for %s: %w", name, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("rulesgen render: write %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("rulesgen render: close %s: %w", name, err)
	}
	if err := os.Rename(tmpName, final); err != nil {
		return fmt.Errorf("rulesgen render: rename to %s: %w", name, err)
	}
	return nil
}

// RenderFragments rewrites watch.yaml, cleanup-onetime.yaml,
// cleanup-ongoing.yaml and ignore.yaml from the full decision state, so the
// files always match the store (handlers re-render after every change).
// Watch rules keep (cluster_id, lane) load order, which is deterministic per
// store. Runs are atomic (temp + rename) so a reader never sees a partial file.
type ignoreAcc struct {
	listIDs       []yamlString
	senderDomains []yamlString
	recipientTags []yamlString
}

func RenderFragments(dir string, st *Store) error {
	decisions, err := st.AllDecisions()
	if err != nil {
		return err
	}
	var lanes []Lane
	for _, l := range []Lane{LaneWatch, LaneOneTimeCleanup, LaneOngoingCleanup} {
		lanes = append(lanes, l)
	}
	rulesByLane := map[Lane][]Rule{}
	ignoreByLane := map[Lane]*ignoreAcc{}

	for _, d := range decisions {
		switch d.Decision {
		case DecisionGenerated:
			var rules []Rule
			if len(d.Payload) > 0 {
				if err := json.Unmarshal(d.Payload, &rules); err != nil {
					return fmt.Errorf("rulesgen render: parse payload for %s:%s: %w", d.ClusterID, d.Lane, err)
				}
			}
			rulesByLane[d.Lane] = append(rulesByLane[d.Lane], rules...)
		case DecisionIgnored:
			var ident ignoreIdentity
			if len(d.Payload) > 0 {
				if err := json.Unmarshal(d.Payload, &ident); err != nil {
					return fmt.Errorf("rulesgen render: parse ignore payload for %s:%s: %w", d.ClusterID, d.Lane, err)
				}
			}
			a := ignoreByLane[d.Lane]
			if a == nil {
				a = &ignoreAcc{}
				ignoreByLane[d.Lane] = a
			}
			// ADR 0002 identities are single-field per decision (one field per
			// lens), so a sender domain only ever lands next to an empty list
			// & tag. Defensive only: drop list/tag if senders are present.
			if len(ident.SenderDomains) > 0 {
				a.senderDomains = append(a.senderDomains, toYamlStrings(ident.SenderDomains)...)
			} else {
				a.listIDs = append(a.listIDs, toYamlStrings(ident.ListIDs)...)
				a.recipientTags = append(a.recipientTags, toYamlStrings(ident.RecipientTags)...)
			}
		}
	}

	for _, lane := range lanes {
		data, err := emitYAML(rulesDoc{Rules: rulesByLane[lane]})
		if err != nil {
			return fmt.Errorf("rulesgen render: %s: %w", lane, err)
		}
		if err := writeFragment(dir, fragmentName(lane), data); err != nil {
			return err
		}
	}

	cleanup := &ignoreAcc{}
	for _, l := range []Lane{LaneOneTimeCleanup, LaneOngoingCleanup} {
		if a := ignoreByLane[l]; a != nil {
			cleanup.listIDs = append(cleanup.listIDs, a.listIDs...)
			cleanup.senderDomains = append(cleanup.senderDomains, a.senderDomains...)
			cleanup.recipientTags = append(cleanup.recipientTags, a.recipientTags...)
		}
	}
	watchSide := toIgnoreSide(ignoreByLane[LaneWatch])
	cleanupSide := toIgnoreSide(cleanup)

	if watchSide == nil && cleanupSide == nil {
		if err := os.Remove(filepath.Join(dir, "ignore.yaml")); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("rulesgen render: remove stale ignore.yaml: %w", err)
		}
		return nil
	}
	data, err := emitYAML(ignoreDoc{Ignore: ignoreSides{Watch: watchSide, Cleanup: cleanupSide}})
	if err != nil {
		return fmt.Errorf("rulesgen render: ignore: %w", err)
	}
	return writeFragment(dir, "ignore.yaml", data)
}

func fragmentName(lane Lane) string {
	switch lane {
	case LaneWatch:
		return "watch.yaml"
	case LaneOneTimeCleanup:
		return "cleanup-onetime.yaml"
	default:
		return "cleanup-ongoing.yaml"
	}
}

func toIgnoreSide(a *ignoreAcc) *ignoreSideMatchers {
	if a == nil {
		return nil
	}
	side := &ignoreSideMatchers{
		ListIDs:       dedupeSort(a.listIDs),
		SenderDomains: dedupeSort(a.senderDomains),
		RecipientTags: dedupeSort(a.recipientTags),
	}
	if len(side.ListIDs) == 0 && len(side.SenderDomains) == 0 && len(side.RecipientTags) == 0 {
		return nil
	}
	return side
}

func dedupeSort(values []yamlString) []yamlString {
	seen := make(map[string]bool, len(values))
	out := make([]yamlString, 0, len(values))
	for _, v := range values {
		if seen[string(v)] {
			continue
		}
		seen[string(v)] = true
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

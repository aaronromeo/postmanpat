package rulesgen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type parityCase struct {
	Kind    string         `json:"kind"`
	Lens    string         `json:"lens"`
	Lane    string         `json:"lane"`
	Side    string         `json:"side"`
	Cluster reportCluster  `json:"cluster"`
	Inputs  GenerateInputs `json:"inputs"`
}

func TestParityCorpus(t *testing.T) {
	cases, err := filepath.Glob(filepath.Join("testdata", "parity", "*.json"))
	require.NoError(t, err)
	require.NotEmpty(t, cases)

	for _, cf := range cases {
		data, err := os.ReadFile(cf)
		require.NoError(t, err)
		var pc parityCase
		require.NoError(t, json.Unmarshal(data, &pc), cf)

		goldenPath := strings.TrimSuffix(cf, ".json") + ".yaml"
		golden, err := os.ReadFile(goldenPath)
		require.NoError(t, err, goldenPath)

		cluster := Cluster{
			ClusterID:  pc.Cluster.ClusterID,
			Lens:       pc.Lens,
			Keys:       pc.Cluster.Keys,
			Count:      pc.Cluster.Count,
			LatestDate: pc.Cluster.LatestDate,
			Examples:   pc.Cluster.Examples,
			Signals:    pc.Cluster.Signals,
			Suppressed: pc.Cluster.Suppressed,
		}

		var got []byte
		switch pc.Kind {
		case "rule":
			rules, err := BuildRules(cluster, Lane(pc.Lane), pc.Inputs)
			require.NoError(t, err, cf)
			got, err = emitYAML(rulesDoc{Rules: rules})
			require.NoError(t, err, cf)
		case "ignore":
			ident := IgnoreIdentity(cluster)
			side := ignoreSideMatchers{
				ListIDs:       toYamlStrings(ident.ListIDs),
				SenderDomains: toYamlStrings(ident.SenderDomains),
				RecipientTags: toYamlStrings(ident.RecipientTags),
			}
			doc := ignoreDoc{Ignore: ignoreSides{}}
			if pc.Side == "watch" {
				doc.Ignore.Watch = &side
			} else {
				doc.Ignore.Cleanup = &side
			}
			got, err = emitYAML(doc)
			require.NoError(t, err, cf)
		default:
			t.Fatalf("unknown corpus kind %q in %s", pc.Kind, cf)
		}

		require.Equal(t, string(golden), string(got), cf)
	}
}

func TestEscapeRegexMatchesPythonReEscape(t *testing.T) {
	cases := map[string]string{
		"my-shop.example.org": `my\-shop\.example\.org`,
		"updates.example.com": `updates\.example\.com`,
		"aaron@example.com":   `aaron@example\.com`,
		"tag-news#1":          `tag\-news\#1`,
		"x~y&z":               `x\~y\&z`,
		"with space":          `with\ space`,
		"plain":               `plain`,
		"a.b[c]d":             `a\.b\[c\]d`,
	}
	for in, want := range cases {
		require.Equal(t, want, escapeRegex(in), in)
	}
}

func TestBuildRulesRequiresLaneForLens(t *testing.T) {
	c := testCluster("c1", "recipient_tag_lens")
	_, err := BuildRules(c, LaneOngoingCleanup, GenerateInputs{Name: "x"})
	require.Error(t, err)
}

func TestBuildRulesRejectsMoveWithoutDestination(t *testing.T) {
	c := testCluster("c1", "list_lens")
	_, err := BuildRules(c, LaneWatch, GenerateInputs{Name: "x", ListIDRegex: "a\\.b", Action: "move"})
	require.Error(t, err)
}

func TestBuildRulesRejectsUnknownAction(t *testing.T) {
	c := testCluster("c1", "list_lens")
	_, err := BuildRules(c, LaneWatch, GenerateInputs{Name: "x", ListIDRegex: "a", Action: "expunge"})
	require.Error(t, err)
}

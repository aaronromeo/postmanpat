package rulesgen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decideGenerated(t *testing.T, st *Store, clusterID string, lane Lane, rule Rule) {
	t.Helper()
	payload, err := json.Marshal([]Rule{rule})
	require.NoError(t, err)
	require.NoError(t, st.Decide(clusterID, lane, DecisionGenerated, payload))
}

func decideIgnored(t *testing.T, st *Store, clusterID string, lane Lane, ident ignoreIdentity) {
	t.Helper()
	payload, err := json.Marshal(ident)
	require.NoError(t, err)
	require.NoError(t, st.Decide(clusterID, lane, DecisionIgnored, payload))
}

func renderAndRead(t *testing.T, dir string, st *Store) map[string]string {
	t.Helper()
	require.NoError(t, RenderFragments(dir, st))
	out := map[string]string{}
	for _, name := range []string{"watch.yaml", "cleanup-ongoing.yaml", "cleanup-onetime.yaml", "ignore.yaml"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if os.IsNotExist(err) {
			out[name] = ""
			continue
		}
		require.NoError(t, err, name)
		out[name] = string(data)
	}
	return out
}

func TestRenderFragmentsSeparatesLanesAndAggregatesIgnore(t *testing.T) {
	st := openTestStore(t)
	dir := t.TempDir()

	watchRule := Rule{
		Name:    "Updates",
		Client:  &watchClient{ListIDRegex: []yamlString{`updates\.example\.com`}},
		Actions: []ruleAction{{Type: "delete"}},
	}
	ongoingRule := Rule{
		Name:    "Shop",
		Server:  &cleanupServer{Folders: []yamlString{"INBOX"}, SenderSubstring: []yamlString{"shop.example.org"}},
		Actions: []ruleAction{{Type: "delete"}},
	}
	decideGenerated(t, st, "list-updates", LaneWatch, watchRule)
	decideGenerated(t, st, "sender-shop", LaneOngoingCleanup, ongoingRule)
	decideIgnored(t, st, "list-some", LaneWatch, ignoreIdentity{ListIDs: []string{"some.list"}})
	decideIgnored(t, st, "list-some", LaneOngoingCleanup, ignoreIdentity{ListIDs: []string{"some.list"}})

	got := renderAndRead(t, dir, st)

	wantWatch, err := emitYAML(rulesDoc{Rules: []Rule{watchRule}})
	require.NoError(t, err)
	assert.Equal(t, string(wantWatch), got["watch.yaml"])
	assert.Equal(t, `rules: []`+"\n", got["cleanup-onetime.yaml"])

	wantIgnore, err := emitYAML(ignoreDoc{Ignore: ignoreSides{
		Watch:   &ignoreSideMatchers{ListIDs: []yamlString{"some.list"}},
		Cleanup: &ignoreSideMatchers{ListIDs: []yamlString{"some.list"}},
	}})
	require.NoError(t, err)
	assert.Equal(t, string(wantIgnore), got["ignore.yaml"])
	assert.Contains(t, got["cleanup-ongoing.yaml"], `name: "Shop"`)
}

func TestRenderFragmentsIgnoreAggregatesSortedDedupedPerField(t *testing.T) {
	st := openTestStore(t)
	dir := t.TempDir()

	decideIgnored(t, st, "a", LaneWatch, ignoreIdentity{ListIDs: []string{"z.list", "a.list"}})
	decideIgnored(t, st, "b", LaneWatch, ignoreIdentity{ListIDs: []string{"m.list", "a.list"}})
	decideIgnored(t, st, "c", LaneWatch, ignoreIdentity{SenderDomains: []string{"x.com"}})

	got := renderAndRead(t, dir, st)

	want, err := emitYAML(ignoreDoc{Ignore: ignoreSides{
		Watch: &ignoreSideMatchers{
			ListIDs:       []yamlString{"a.list", "m.list", "z.list"},
			SenderDomains: []yamlString{"x.com"},
		},
	}})
	require.NoError(t, err)
	assert.Equal(t, string(want), got["ignore.yaml"])
	assert.Equal(t, `rules: []`+"\n", got["watch.yaml"])
}

func TestRenderFragmentsOmitEmptySidesAndNoIgnoreWhenEmpty(t *testing.T) {
	st := openTestStore(t)
	dir := t.TempDir()

	decideIgnored(t, st, "c", LaneOneTimeCleanup, ignoreIdentity{SenderDomains: []string{"x.com"}})

	got := renderAndRead(t, dir, st)

	want, err := emitYAML(ignoreDoc{Ignore: ignoreSides{
		Cleanup: &ignoreSideMatchers{SenderDomains: []yamlString{"x.com"}},
	}})
	require.NoError(t, err)
	assert.Equal(t, string(want), got["ignore.yaml"])
	assert.Equal(t, `rules: []`+"\n", got["watch.yaml"])
	assert.Equal(t, `rules: []`+"\n", got["cleanup-ongoing.yaml"])
}

func TestRenderFragmentsRemovesIgnoreWhenAllReverted(t *testing.T) {
	st := openTestStore(t)
	dir := t.TempDir()

	decideIgnored(t, st, "c", LaneWatch, ignoreIdentity{ListIDs: []string{"some.list"}})
	require.NoError(t, RenderFragments(dir, st))
	require.FileExists(t, filepath.Join(dir, "ignore.yaml"))

	require.NoError(t, st.Revert("c", LaneWatch))
	require.NoError(t, RenderFragments(dir, st))
	assert.NoFileExists(t, filepath.Join(dir, "ignore.yaml"))
}

func TestRenderFragmentsIsIdempotentByteWise(t *testing.T) {
	st := openTestStore(t)
	dir := t.TempDir()
	decideGenerated(t, st, "c1", LaneWatch, Rule{Name: "A", Client: &watchClient{ListIDRegex: []yamlString{`a\.b`}}, Actions: []ruleAction{{Type: "delete"}}})
	decideIgnored(t, st, "c2", LaneWatch, ignoreIdentity{RecipientTags: []string{"news"}})

	first := renderAndRead(t, dir, st)
	second := renderAndRead(t, dir, st)

	assert.Equal(t, first, second)
}

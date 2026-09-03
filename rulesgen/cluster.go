package rulesgen

type Examples struct {
	SubjectRaw             []string `json:"subject_raw"`
	Recipients             []string `json:"recipients"`
	ReplyToDomains         []string `json:"reply_to_domains"`
	SenderDomains          []string `json:"sender_domains"`
	ReturnPathDomains      []string `json:"returnpath_domains"`
	ListUnsubscribeTargets []string `json:"list_unsubscribe_targets"`
}

type Signals struct {
	HasListID            bool           `json:"has_list_id"`
	HasListUnsubscribe   bool           `json:"has_list_unsubscribe"`
	PrecedenceCategories map[string]int `json:"precedence_categories"`
}

type Cluster struct {
	ClusterID  string         `json:"cluster_id"`
	Lens       string         `json:"lens"`
	Keys       map[string]any `json:"keys"`
	Count      int            `json:"count"`
	LatestDate string         `json:"latest_date"`
	Examples   Examples       `json:"examples"`
	Signals    Signals        `json:"signals"`
	Suppressed []string       `json:"suppressed"`
	FirstSeen  string         `json:"first_seen"`
	LastSeen   string         `json:"last_seen"`
}

func (c Cluster) suppressedForBoth() bool {
	watch, cleanup := false, false
	for _, s := range c.Suppressed {
		if s == "watch" {
			watch = true
		}
		if s == "cleanup" {
			cleanup = true
		}
	}
	return watch && cleanup
}

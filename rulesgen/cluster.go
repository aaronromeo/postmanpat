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
	ClusterID  string
	Lens       string
	Keys       map[string]any
	Count      int
	LatestDate string
	Examples   Examples
	Signals    Signals
	Suppressed []string
	FirstSeen  string
	LastSeen   string
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

package rulesgen

type report struct {
	GeneratedAt string `json:"generated_at"`
	Indexes     struct {
		ListLens         reportLens `json:"list_lens"`
		SenderUnsubLens  reportLens `json:"sender_unsub_lens"`
		TemplateLens     reportLens `json:"template_lens"`
		RecipientTagLens reportLens `json:"recipient_tag_lens"`
	} `json:"indexes"`
}

type reportLens struct {
	Clusters []reportCluster `json:"clusters"`
}

type reportCluster struct {
	ClusterID  string         `json:"cluster_id"`
	Count      int            `json:"count"`
	LatestDate string         `json:"latest_date"`
	Keys       map[string]any `json:"keys"`
	Signals    Signals        `json:"signals"`
	Examples   Examples       `json:"examples"`
	Suppressed []string       `json:"suppressed"`
}

func (r report) clusters() []Cluster {
	var out []Cluster
	for _, lens := range []struct {
		name string
		lens reportLens
	}{
		{"list_lens", r.Indexes.ListLens},
		{"sender_unsub_lens", r.Indexes.SenderUnsubLens},
		{"recipient_tag_lens", r.Indexes.RecipientTagLens},
	} {
		for _, rc := range lens.lens.Clusters {
			out = append(out, Cluster{
				ClusterID:  rc.ClusterID,
				Lens:       lens.name,
				Keys:       rc.Keys,
				Count:      rc.Count,
				LatestDate: rc.LatestDate,
				Examples:   rc.Examples,
				Signals:    rc.Signals,
				Suppressed: rc.Suppressed,
				LastSeen:   r.GeneratedAt,
			})
		}
	}
	return out
}

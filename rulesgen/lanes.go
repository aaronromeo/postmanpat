package rulesgen

type Lane string

const (
	LaneWatch          Lane = "watch"
	LaneOneTimeCleanup Lane = "one_time_cleanup"
	LaneOngoingCleanup Lane = "ongoing_cleanup"
)

var lensLanes = map[string][]Lane{
	"list_lens":          {LaneWatch, LaneOneTimeCleanup, LaneOngoingCleanup},
	"sender_unsub_lens":  {LaneWatch, LaneOneTimeCleanup, LaneOngoingCleanup},
	"recipient_tag_lens": {LaneWatch},
}

package jellyfin

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/sj14/jellyfin-go/api"
)

// Helpers to try to contain BaseItemDto implementation from leaking out of the package without casting it to something else
// NOTE: just cast it to something else?

// Item is a type alias just because it looks nicer
type Item = api.BaseItemDto

func getItemRuntime(ticks int64) string {
	minutes := ticks / 600_000_000
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}
	hours := minutes / 60
	minutes -= hours * 60
	return fmt.Sprintf("%dh%dm", hours, minutes)
}

// GetStreamingURL builds the playback URL for an item. The capitalized route
// and the api_key are what the iina-jellyfin plugin matches on to recognize the
// item and report playback against it.
func GetStreamingURL(host, token string, item Item) (string, error) {
	host, err := NormalizeHost(host)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/Videos/%s/stream?static=true&api_key=%s", host, *item.Id, url.QueryEscape(token)), nil
}

func GetItemTitle(item Item) string {
	str := &strings.Builder{}
	switch item.GetType() {
	case api.BASEITEMKIND_MOVIE:
		fmt.Fprintf(str, "%s (%d)", item.GetName(), item.GetProductionYear())
		if data, ok := item.GetUserDataOk(); ok && data.GetPlayedPercentage() > 0 {
			fmt.Fprintf(str, " [%.f%%]", data.GetPlayedPercentage())
		}
	case api.BASEITEMKIND_EPISODE:
		fmt.Fprintf(str, "%s S%.2dE%.2d (%d)", item.GetSeriesName(), item.GetParentIndexNumber(), item.GetIndexNumber(), item.GetProductionYear())
		if data, ok := item.GetUserDataOk(); ok && data.GetPlayedPercentage() > 0 {
			fmt.Fprintf(str, " [%.f%%]", data.GetPlayedPercentage())
		}
	case api.BASEITEMKIND_SERIES:
		fmt.Fprintf(str, "%s (%d)", item.GetName(), item.GetProductionYear())
		if data, ok := item.GetUserDataOk(); ok {
			fmt.Fprintf(str, " [%d]", data.GetUnplayedItemCount())
		}
	case api.BASEITEMKIND_VIDEO:
		fmt.Fprintf(str, "%s (%d)", item.GetName(), item.GetProductionYear())
	}
	return str.String()
}

func GetItemDescription(item Item) string {
	str := &strings.Builder{}
	switch item.GetType() {
	case api.BASEITEMKIND_MOVIE:
		fmt.Fprintf(str, "Movie  | Rating: %.1f | Runtime: %s", item.GetCommunityRating(), getItemRuntime(item.GetRunTimeTicks()))
	case api.BASEITEMKIND_SERIES:
		fmt.Fprintf(str, "Series | Rating: %.1f", item.GetCommunityRating())
	case api.BASEITEMKIND_EPISODE:
		fmt.Fprintf(str, "%s", item.GetName())
	case api.BASEITEMKIND_VIDEO:
		fmt.Fprintf(str, "Video  | Rating: %.1f | Runtime: %s", item.GetCommunityRating(), getItemRuntime(item.GetRunTimeTicks()))
	}
	return str.String()
}

func IsSeries(item Item) bool {
	return item.GetType() == api.BASEITEMKIND_SERIES
}

func IsEpisode(item Item) bool {
	return item.GetType() == api.BASEITEMKIND_EPISODE
}

func Watched(item Item) bool {
	if data, ok := item.GetUserDataOk(); ok {
		return data.GetPlayed()
	}
	return false
}

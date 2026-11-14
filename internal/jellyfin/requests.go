package jellyfin

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/sj14/jellyfin-go/api"
)

func itemQueryResult(operation string, res *api.BaseItemDtoQueryResult, response *http.Response, err error) ([]Item, error) {
	if err != nil {
		attrs := []any{"operation", operation, "error_type", fmt.Sprintf("%T", err)}
		if response != nil {
			attrs = append(attrs, "status_code", response.StatusCode)
		}
		slog.Error("item query failed", attrs...)
		return nil, err
	}
	slog.Debug("item query completed", "operation", operation, "item_count", len(res.Items))
	return res.Items, nil
}

func (c *Client) GetResume(userID string) ([]Item, error) {
	res, response, err := c.ItemsAPI.GetResumeItems(context.Background()).
		UserId(userID).
		IncludeItemTypes([]api.BaseItemKind{api.BASEITEMKIND_MOVIE, api.BASEITEMKIND_EPISODE, api.BASEITEMKIND_VIDEO}).
		Fields([]api.ItemFields{api.ITEMFIELDS_MEDIA_STREAMS}).
		Execute()
	return itemQueryResult("resume", res, response, err)
}

func (c *Client) GetNextUp() ([]Item, error) {
	res, response, err := c.TvShowsAPI.GetNextUp(context.Background()).
		Fields([]api.ItemFields{api.ITEMFIELDS_MEDIA_STREAMS}).
		EnableTotalRecordCount(false).
		DisableFirstEpisode(false).
		EnableResumable(false).
		EnableRewatching(false).
		Execute()
	return itemQueryResult("next_up", res, response, err)
}

func (c *Client) GetRecentlyAdded() ([]Item, error) {
	res, response, err := c.ItemsAPI.GetItems(context.Background()).
		Recursive(true).
		IncludeItemTypes([]api.BaseItemKind{api.BASEITEMKIND_MOVIE, api.BASEITEMKIND_SERIES, api.BASEITEMKIND_VIDEO}).
		Fields([]api.ItemFields{api.ITEMFIELDS_MEDIA_STREAMS}).
		Limit(100).
		SortBy([]api.ItemSortBy{api.ITEMSORTBY_DATE_CREATED}).
		SortOrder([]api.SortOrder{api.SORTORDER_DESCENDING}).
		Execute()
	return itemQueryResult("recently_added", res, response, err)
}

func (c *Client) GetEpisodes(item Item) ([]Item, error) {
	seriesID := item.GetSeriesId()
	if item.GetType() == api.BASEITEMKIND_SERIES {
		seriesID = item.GetId()
	}
	res, response, err := c.TvShowsAPI.GetEpisodes(context.Background(), seriesID).
		Fields([]api.ItemFields{api.ITEMFIELDS_MEDIA_STREAMS}).
		Execute()
	return itemQueryResult("episodes", res, response, err)
}

func (c *Client) Search(query string) ([]Item, error) {
	res, response, err := c.ItemsAPI.GetItems(context.Background()).
		SearchTerm(query).
		Recursive(true).
		IncludeItemTypes([]api.BaseItemKind{api.BASEITEMKIND_MOVIE, api.BASEITEMKIND_SERIES, api.BASEITEMKIND_VIDEO}).
		Fields([]api.ItemFields{api.ITEMFIELDS_MEDIA_STREAMS}).
		Limit(100).
		Execute()
	return itemQueryResult("search", res, response, err)
}

func (c *Client) MarkAsWatched(item Item) error {
	_, _, err := c.PlaystateAPI.MarkPlayedItem(context.Background(), item.GetId()).Execute()
	return err
}

func (c *Client) MarkAsUnwatched(item Item) error {
	_, _, err := c.PlaystateAPI.MarkUnplayedItem(context.Background(), item.GetId()).Execute()
	return err
}

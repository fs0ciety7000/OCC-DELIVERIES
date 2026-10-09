package domain

import "strings"

// HistoryLine is one of my order lines in a party (stats input).
type HistoryLine struct {
	Name     string
	Quantity int
}

// HistoryOrder is my participation in one party, newest first in the input
// slice of ComputeHistoryStats (the index is used as recency tie-break).
type HistoryOrder struct {
	Status          string
	RestaurantID    string
	RestaurantName  string
	RestaurantEmoji string
	// Total is my share (items + shared fees), in cents.
	Total int
	Lines []HistoryLine
}

// FavoriteRestaurant is the restaurant I ordered from the most.
type FavoriteRestaurant struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Emoji  string `json:"emoji"`
	Orders int    `json:"orders"`
}

// FavoriteDish is the dish I ordered the most (summed quantities).
type FavoriteDish struct {
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
	Orders   int    `json:"orders"`
}

// HistoryStats summarises a user's past orders (GET /api/occ/me/stats).
type HistoryStats struct {
	Orders             int                 `json:"orders"`
	TotalSpent         int                 `json:"totalSpent"`
	FavoriteRestaurant *FavoriteRestaurant `json:"favoriteRestaurant"`
	FavoriteDish       *FavoriteDish       `json:"favoriteDish"`
}

// placedStatuses are the statuses of an order that was actually placed
// (baskets locked): ongoing lobby/voting/ordering and cancelled ones don't count.
var placedStatuses = map[string]bool{StatusReview: true, StatusPaying: true, StatusClosed: true}

// CountsAsOrder tells whether a participation is a placed order of mine.
func CountsAsOrder(o HistoryOrder) bool {
	if !placedStatuses[o.Status] {
		return false
	}
	for _, l := range o.Lines {
		if l.Quantity > 0 {
			return true
		}
	}
	return false
}

// ComputeHistoryStats aggregates placed orders. Input is newest first.
// Ties: more orders (or quantity) first, then the most recent, then the name.
func ComputeHistoryStats(orders []HistoryOrder) HistoryStats {
	type restAgg struct {
		fav    FavoriteRestaurant
		recent int
	}
	type dishAgg struct {
		fav    FavoriteDish
		recent int
	}
	st := HistoryStats{}
	rests := map[string]*restAgg{}
	dishes := map[string]*dishAgg{}

	for i, o := range orders {
		if !CountsAsOrder(o) {
			continue
		}
		st.Orders++
		st.TotalSpent += max(o.Total, 0)
		if o.RestaurantID != "" {
			ra, found := rests[o.RestaurantID]
			if !found {
				ra = &restAgg{fav: FavoriteRestaurant{ID: o.RestaurantID, Name: o.RestaurantName, Emoji: o.RestaurantEmoji}, recent: i}
				rests[o.RestaurantID] = ra
			}
			ra.fav.Orders++
		}
		seen := map[string]bool{}
		for _, l := range o.Lines {
			key := strings.ToLower(strings.TrimSpace(l.Name))
			if key == "" || l.Quantity <= 0 {
				continue
			}
			da, found := dishes[key]
			if !found {
				da = &dishAgg{fav: FavoriteDish{Name: strings.TrimSpace(l.Name)}, recent: i}
				dishes[key] = da
			}
			da.fav.Quantity += l.Quantity
			if !seen[key] {
				seen[key] = true
				da.fav.Orders++
			}
		}
	}

	var bestR *restAgg
	for _, ra := range rests {
		switch {
		case bestR == nil,
			ra.fav.Orders > bestR.fav.Orders,
			ra.fav.Orders == bestR.fav.Orders && ra.recent < bestR.recent,
			ra.fav.Orders == bestR.fav.Orders && ra.recent == bestR.recent && ra.fav.Name < bestR.fav.Name:
			bestR = ra
		}
	}
	if bestR != nil {
		f := bestR.fav
		st.FavoriteRestaurant = &f
	}

	var bestD *dishAgg
	for _, da := range dishes {
		switch {
		case bestD == nil,
			da.fav.Quantity > bestD.fav.Quantity,
			da.fav.Quantity == bestD.fav.Quantity && da.fav.Orders > bestD.fav.Orders,
			da.fav.Quantity == bestD.fav.Quantity && da.fav.Orders == bestD.fav.Orders && da.recent < bestD.recent,
			da.fav.Quantity == bestD.fav.Quantity && da.fav.Orders == bestD.fav.Orders && da.recent == bestD.recent && da.fav.Name < bestD.fav.Name:
			bestD = da
		}
	}
	if bestD != nil {
		f := bestD.fav
		st.FavoriteDish = &f
	}
	return st
}

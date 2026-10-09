package domain

import "testing"

func TestComputeHistoryStats(t *testing.T) {
	pizza := func(status string, total int, lines ...HistoryLine) HistoryOrder {
		return HistoryOrder{Status: status, RestaurantID: "p", RestaurantName: "Pizza", RestaurantEmoji: "🍕", Total: total, Lines: lines}
	}
	sushi := func(status string, total int, lines ...HistoryLine) HistoryOrder {
		return HistoryOrder{Status: status, RestaurantID: "s", RestaurantName: "Sushi", RestaurantEmoji: "🍣", Total: total, Lines: lines}
	}
	tests := []struct {
		name      string
		in        []HistoryOrder
		orders    int
		spent     int
		rest      string
		restCount int
		dish      string
		dishQty   int
	}{
		{name: "vide", in: nil},
		{
			name: "seules les commandes passées comptent",
			in: []HistoryOrder{
				pizza(StatusOrdering, 1000, HistoryLine{"Margherita", 1}),
				pizza(StatusCancelled, 1000, HistoryLine{"Margherita", 1}),
				pizza(StatusLobby, 0),
				sushi(StatusClosed, 1450, HistoryLine{"Maki", 2}),
				pizza(StatusClosed, 0), // membre sans article
			},
			orders: 1, spent: 1450, rest: "s", restCount: 1, dish: "Maki", dishQty: 2,
		},
		{
			name: "restaurant le plus fréquent, plat le plus commandé (quantités)",
			in: []HistoryOrder{
				sushi(StatusPaying, 2000, HistoryLine{"Maki", 1}),
				pizza(StatusClosed, 1200, HistoryLine{"Margherita", 1}, HistoryLine{"Tiramisu", 1}),
				pizza(StatusReview, 1300, HistoryLine{"margherita ", 2}),
			},
			orders: 3, spent: 4500, rest: "p", restCount: 2, dish: "Margherita", dishQty: 3,
		},
		{
			name: "égalité : le plus récent gagne",
			in: []HistoryOrder{
				sushi(StatusClosed, 900, HistoryLine{"Maki", 1}),
				pizza(StatusClosed, 800, HistoryLine{"Calzone", 1}),
			},
			orders: 2, spent: 1700, rest: "s", restCount: 1, dish: "Maki", dishQty: 1,
		},
		{
			name:   "montant négatif ignoré",
			in:     []HistoryOrder{pizza(StatusClosed, -5, HistoryLine{"Calzone", 1})},
			orders: 1, spent: 0, rest: "p", restCount: 1, dish: "Calzone", dishQty: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := ComputeHistoryStats(tt.in)
			if st.Orders != tt.orders || st.TotalSpent != tt.spent {
				t.Fatalf("orders/spent = %d/%d, want %d/%d", st.Orders, st.TotalSpent, tt.orders, tt.spent)
			}
			if tt.rest == "" {
				if st.FavoriteRestaurant != nil || st.FavoriteDish != nil {
					t.Fatalf("expected no favourites: %+v", st)
				}
				return
			}
			if st.FavoriteRestaurant == nil || st.FavoriteRestaurant.ID != tt.rest || st.FavoriteRestaurant.Orders != tt.restCount {
				t.Fatalf("favourite restaurant = %+v, want %s×%d", st.FavoriteRestaurant, tt.rest, tt.restCount)
			}
			if st.FavoriteDish == nil || st.FavoriteDish.Name != tt.dish || st.FavoriteDish.Quantity != tt.dishQty {
				t.Fatalf("favourite dish = %+v, want %s×%d", st.FavoriteDish, tt.dish, tt.dishQty)
			}
		})
	}
}

package domain

// UserInfo is the public identity of a participant.
type UserInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
	Color  string `json:"color"`
}

// MemberInfo is a party member as seen by the summary builder.
type MemberInfo struct {
	User  UserInfo
	Ready bool
}

// ItemInfo is an order line as stored (prices already computed server side).
type ItemInfo struct {
	ID           string
	UserID       string
	MenuItem     string
	Name         string
	OptionsLabel string
	OptionsKey   string
	Note         string
	Quantity     int
	UnitPrice    int
	Total        int
}

// RestaurantInfo is the subset of restaurant data exposed in the summary.
type RestaurantInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	MinOrder    int    `json:"minOrder"`
	DeliveryFee int    `json:"deliveryFee"`
}

// SummaryInput gathers everything needed to compute a party summary.
type SummaryInput struct {
	PartyID     string
	Status      string
	SplitMode   string
	Restaurant  *RestaurantInfo
	DeliveryFee int
	ServiceFee  int
	Tip         int
	// Members in display order (host first is recommended).
	Members []MemberInfo
	// Items in creation order.
	Items []ItemInfo
}

// SummaryItem is a participant's line in the summary.
type SummaryItem struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	OptionsLabel string `json:"optionsLabel"`
	Note         string `json:"note"`
	Quantity     int    `json:"quantity"`
	UnitPrice    int    `json:"unitPrice"`
	Total        int    `json:"total"`
}

// Participant is one member's share.
type Participant struct {
	User       UserInfo      `json:"user"`
	Ready      bool          `json:"ready"`
	Items      []SummaryItem `json:"items"`
	Subtotal   int           `json:"subtotal"`
	SharedFees int           `json:"sharedFees"`
	Total      int           `json:"total"`
}

// ConsolidatedLine merges identical lines (same menu item and options).
type ConsolidatedLine struct {
	MenuItem     string   `json:"menuItem"`
	Name         string   `json:"name"`
	OptionsLabel string   `json:"optionsLabel"`
	Quantity     int      `json:"quantity"`
	Total        int      `json:"total"`
	Notes        []string `json:"notes"`
}

// Summary is the full recap of a party (see docs/ARCHITECTURE.md).
type Summary struct {
	PartyID         string             `json:"partyId"`
	Currency        string             `json:"currency"`
	Status          string             `json:"status"`
	Restaurant      *RestaurantInfo    `json:"restaurant"`
	Participants    []Participant      `json:"participants"`
	Consolidated    []ConsolidatedLine `json:"consolidated"`
	ItemsSubtotal   int                `json:"itemsSubtotal"`
	DeliveryFee     int                `json:"deliveryFee"`
	ServiceFee      int                `json:"serviceFee"`
	Tip             int                `json:"tip"`
	SharedFees      int                `json:"sharedFees"`
	GrandTotal      int                `json:"grandTotal"`
	MinOrderReached bool               `json:"minOrderReached"`
	AllReady        bool               `json:"allReady"`
	SplitMode       string             `json:"splitMode"`
}

// ParticipantByID returns the participant of the given user, if any.
func (s Summary) ParticipantByID(userID string) (Participant, bool) {
	for _, p := range s.Participants {
		if p.User.ID == userID {
			return p, true
		}
	}
	return Participant{}, false
}

// BuildSummary computes totals, fee shares and the consolidated cart.
// Invariant: Σ participants.total == grandTotal.
func BuildSummary(in SummaryInput) Summary {
	mode := in.SplitMode
	if mode != SplitProportional {
		mode = SplitEqual
	}
	s := Summary{
		PartyID:      in.PartyID,
		Currency:     "EUR",
		Status:       in.Status,
		Restaurant:   in.Restaurant,
		Participants: []Participant{},
		Consolidated: []ConsolidatedLine{},
		DeliveryFee:  max(in.DeliveryFee, 0),
		ServiceFee:   max(in.ServiceFee, 0),
		Tip:          max(in.Tip, 0),
		SplitMode:    mode,
	}
	s.SharedFees = s.DeliveryFee + s.ServiceFee + s.Tip

	idx := map[string]int{}
	for _, m := range in.Members {
		if _, ok := idx[m.User.ID]; ok {
			continue
		}
		idx[m.User.ID] = len(s.Participants)
		s.Participants = append(s.Participants, Participant{User: m.User, Ready: m.Ready, Items: []SummaryItem{}})
	}

	consIdx := map[string]int{}
	for _, it := range in.Items {
		pi, ok := idx[it.UserID]
		if !ok { // orphan line: still accounted for so totals stay exact
			idx[it.UserID] = len(s.Participants)
			pi = idx[it.UserID]
			s.Participants = append(s.Participants, Participant{User: UserInfo{ID: it.UserID}, Items: []SummaryItem{}})
		}
		p := &s.Participants[pi]
		p.Items = append(p.Items, SummaryItem{
			ID: it.ID, Name: it.Name, OptionsLabel: it.OptionsLabel, Note: it.Note,
			Quantity: it.Quantity, UnitPrice: it.UnitPrice, Total: it.Total,
		})
		p.Subtotal += it.Total
		s.ItemsSubtotal += it.Total

		key := it.MenuItem + "|" + it.OptionsKey
		if it.OptionsKey == "" {
			key = it.MenuItem + "|label:" + it.OptionsLabel
		}
		ci, ok := consIdx[key]
		if !ok {
			ci = len(s.Consolidated)
			consIdx[key] = ci
			s.Consolidated = append(s.Consolidated, ConsolidatedLine{
				MenuItem: it.MenuItem, Name: it.Name, OptionsLabel: it.OptionsLabel, Notes: []string{},
			})
		}
		c := &s.Consolidated[ci]
		c.Quantity += it.Quantity
		c.Total += it.Total
		if it.Note != "" {
			c.Notes = append(c.Notes, it.Note)
		}
	}

	var shares []Share
	for _, p := range s.Participants {
		if len(p.Items) > 0 {
			shares = append(shares, Share{ID: p.User.ID, Subtotal: p.Subtotal})
		}
	}
	split := SplitFees(s.SharedFees, mode, shares)

	s.AllReady = len(shares) > 0
	for i := range s.Participants {
		p := &s.Participants[i]
		p.SharedFees = split[p.User.ID]
		p.Total = p.Subtotal + p.SharedFees
		if len(p.Items) > 0 && !p.Ready {
			s.AllReady = false
		}
	}

	if len(shares) > 0 {
		s.GrandTotal = s.ItemsSubtotal + s.SharedFees
	} else {
		// nobody ordered: no fee can be attributed
		s.GrandTotal = s.ItemsSubtotal
	}

	minOrder := 0
	if in.Restaurant != nil {
		minOrder = in.Restaurant.MinOrder
	}
	s.MinOrderReached = in.Restaurant != nil && s.ItemsSubtotal > 0 && s.ItemsSubtotal >= minOrder

	return s
}

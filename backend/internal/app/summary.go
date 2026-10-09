package app

import (
	"sort"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/domain"
	"github.com/fs0ciety7000/occ-deliveries/backend/internal/providers"
)

func partyMembers(app core.App, partyID string) ([]*core.Record, error) {
	recs, err := app.FindAllRecords(colPartyMembers, dbx.HashExp{"party": partyID})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(recs, func(i, j int) bool {
		hi, hj := recs[i].GetString("role") == "host", recs[j].GetString("role") == "host"
		if hi != hj {
			return hi
		}
		ci, cj := recs[i].GetDateTime("created"), recs[j].GetDateTime("created")
		if !ci.Equal(cj) {
			return ci.Before(cj)
		}
		return recs[i].Id < recs[j].Id
	})
	return recs, nil
}

func orderItems(app core.App, partyID string) ([]*core.Record, error) {
	recs, err := app.FindAllRecords(colOrderItems, dbx.HashExp{"party": partyID})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(recs, func(i, j int) bool {
		ci, cj := recs[i].GetDateTime("created"), recs[j].GetDateTime("created")
		if !ci.Equal(cj) {
			return ci.Before(cj)
		}
		return recs[i].Id < recs[j].Id
	})
	return recs, nil
}

func userInfo(u *core.Record) domain.UserInfo {
	name := u.GetString("name")
	if name == "" {
		name = "Membre"
	}
	return domain.UserInfo{ID: u.Id, Name: name, Avatar: u.GetString("avatar"), Color: u.GetString("color")}
}

// estimatedFeeStatuses use the restaurant delivery fee as an estimate since
// the snapshot is only taken when entering "review".
var estimatedFeeStatuses = map[string]bool{
	domain.StatusLobby: true, domain.StatusVoting: true, domain.StatusOrdering: true,
}

// buildSummary loads everything needed and computes the party summary.
func buildSummary(app core.App, party *core.Record) (domain.Summary, *core.Record, error) {
	status := party.GetString("status")

	var rest *core.Record
	var restInfo *domain.RestaurantInfo
	if rid := party.GetString("restaurant"); rid != "" {
		if r, err := app.FindRecordById(colRestaurants, rid); err == nil {
			rest = r
			restInfo = &domain.RestaurantInfo{
				ID: r.Id, Name: r.GetString("name"),
				MinOrder: r.GetInt("min_order"), DeliveryFee: r.GetInt("delivery_fee"),
			}
		}
	}

	deliveryFee := party.GetInt("delivery_fee")
	if estimatedFeeStatuses[status] && rest != nil {
		deliveryFee = rest.GetInt("delivery_fee")
	}

	pms, err := partyMembers(app, party.Id)
	if err != nil {
		return domain.Summary{}, nil, err
	}
	items, err := orderItems(app, party.Id)
	if err != nil {
		return domain.Summary{}, nil, err
	}

	userIDs := make([]string, 0, len(pms))
	for _, pm := range pms {
		userIDs = append(userIDs, pm.GetString("user"))
	}
	users, err := app.FindRecordsByIds(colUsers, userIDs)
	if err != nil {
		return domain.Summary{}, nil, err
	}
	byID := make(map[string]*core.Record, len(users))
	for _, u := range users {
		byID[u.Id] = u
	}

	in := domain.SummaryInput{
		PartyID:     party.Id,
		Status:      status,
		SplitMode:   party.GetString("split_mode"),
		Restaurant:  restInfo,
		DeliveryFee: deliveryFee,
		ServiceFee:  party.GetInt("service_fee"),
		Tip:         party.GetInt("tip"),
	}
	for _, pm := range pms {
		uid := pm.GetString("user")
		ui := domain.UserInfo{ID: uid, Name: "Membre"}
		if u, ok := byID[uid]; ok {
			ui = userInfo(u)
		}
		in.Members = append(in.Members, domain.MemberInfo{User: ui, Ready: pm.GetBool("ready")})
	}
	for _, it := range items {
		var sel []domain.SelectedOption
		_ = it.UnmarshalJSONField("selected_options", &sel)
		in.Items = append(in.Items, domain.ItemInfo{
			ID:           it.Id,
			UserID:       it.GetString("user"),
			MenuItem:     it.GetString("menu_item"),
			Name:         it.GetString("name"),
			OptionsLabel: it.GetString("options_label"),
			OptionsKey:   domain.SelectionKey(sel),
			Note:         it.GetString("note"),
			Quantity:     it.GetInt("quantity"),
			UnitPrice:    it.GetInt("unit_price"),
			Total:        it.GetInt("total"),
		})
	}
	return domain.BuildSummary(in), rest, nil
}

func providerRestaurant(r *core.Record) providers.Restaurant {
	if r == nil {
		return providers.Restaurant{}
	}
	var links []providers.Link
	_ = r.UnmarshalJSONField("providers", &links)
	return providers.Restaurant{
		ID: r.Id, Name: r.GetString("name"), Address: r.GetString("address"),
		Phone: r.GetString("phone"), Providers: links,
	}
}

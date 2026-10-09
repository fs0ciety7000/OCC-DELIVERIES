package domain

// Visibility of restaurants with an incomplete menu (« cartes incomplètes »).
//
// The admin chooses a minimum number of available menu items
// (app_settings.min_menu_items) : public listings hide the restaurants below
// it. 0 disables the filter. Hiding is computed at read time, never stored on
// the restaurant, so it follows the menus automatically (sync, import, admin
// edits) and is reversed by lowering the threshold.

// MaxMinMenuItems is the highest accepted threshold.
const MaxMinMenuItems = 100

// DefaultMinMenuItems is the threshold suggested by the admin UI when the
// filter is enabled.
const DefaultMinMenuItems = 10

// ValidateMinMenuItems checks a threshold (0 = disabled, 1–100).
func ValidateMinMenuItems(n int) error {
	if n < 0 || n > MaxMinMenuItems {
		return Errf("Le nombre minimum de plats doit être compris entre 0 et %d.", MaxMinMenuItems)
	}
	return nil
}

// HiddenIncomplete reports whether a restaurant with itemsCount available
// items is hidden from the public listings for the threshold minItems.
func HiddenIncomplete(itemsCount, minItems int) bool {
	return minItems > 0 && itemsCount < minItems
}

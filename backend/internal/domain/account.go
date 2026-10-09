package domain

import (
	"strings"
	"unicode/utf8"
)

// Account moderation (ban / anonymisation) rules. Pure: the PocketBase glue
// lives in internal/app/accounts.go.

const (
	// DeletedAccountName replaces the name of an anonymised account; its
	// history (order lines, payments) keeps the amounts under this label.
	DeletedAccountName = "Compte supprimé"
	// DeleteConfirmation is the word to type to delete one's own account.
	DeleteConfirmation = "SUPPRIMER"
	// MaxBanReason bounds users.banned_reason.
	MaxBanReason = 300
	// anonymizedDomain uses the reserved .invalid TLD (RFC 2606): no mail can
	// ever be delivered there and it never collides with a real address.
	anonymizedDomain = "occ.invalid"
)

// AnonymizedEmail is the unique placeholder e-mail of a deleted account.
func AnonymizedEmail(userID string) string {
	return "deleted-" + strings.ToLower(userID) + "@" + anonymizedDomain
}

// IsAnonymizedEmail reports whether the address is an AnonymizedEmail.
func IsAnonymizedEmail(email string) bool {
	e := strings.ToLower(strings.TrimSpace(email))
	return strings.HasPrefix(e, "deleted-") && strings.HasSuffix(e, "@"+anonymizedDomain)
}

// NormalizeBanReason trims the reason (optional) and checks its length.
func NormalizeBanReason(s string) (string, error) {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > MaxBanReason {
		return "", Errf("Le motif est trop long (%d caractères maximum).", MaxBanReason)
	}
	return s, nil
}

// CheckDeleteConfirmation validates the typed confirmation (case and
// surrounding spaces ignored).
func CheckDeleteConfirmation(typed string) error {
	if !strings.EqualFold(strings.TrimSpace(typed), DeleteConfirmation) {
		return Errf("Tape %s pour confirmer la suppression de ton compte.", DeleteConfirmation)
	}
	return nil
}

// ModerationAction is a destructive admin action on an account.
type ModerationAction string

const (
	ActionBan    ModerationAction = "ban"
	ActionDelete ModerationAction = "delete"
	ActionDemote ModerationAction = "demote"
)

// ModerationTarget describes the account an action applies to.
type ModerationTarget struct {
	ID      string
	IsAdmin bool
	Banned  bool
	Deleted bool
}

// CheckModeration enforces the safety rules of ban / delete / demote:
// nobody acts on their own account through the admin panel, and the last
// active (not banned, not deleted) admin is never removed. activeAdmins
// counts the active admins, the target included.
func CheckModeration(action ModerationAction, actorID string, t ModerationTarget, activeAdmins int) error {
	if t.Deleted {
		return Errf("Ce compte a déjà été supprimé.")
	}
	if actorID != "" && actorID == t.ID {
		switch action {
		case ActionBan:
			return Errf("Tu ne peux pas suspendre ton propre compte.")
		case ActionDelete:
			return Errf("Supprime ton propre compte depuis ton profil, pas depuis l'administration.")
		default:
			return Errf("Tu ne peux pas retirer tes propres droits d'administration.")
		}
	}
	if t.IsAdmin && !t.Banned && activeAdmins <= 1 {
		return Errf("C'est le dernier compte administrateur actif : nomme d'abord un autre admin.")
	}
	return nil
}

// CheckSelfDelete refuses the self-service deletion while the user still
// hosts an open order, pays for one not settled yet, or is the last admin.
func CheckSelfDelete(isAdmin bool, activeAdmins, openHosted, unsettledAsPayer int) error {
	if openHosted > 0 {
		return Errf("Tu es l'hôte d'une commande en cours : clôture-la ou annule-la avant de supprimer ton compte.")
	}
	if unsettledAsPayer > 0 {
		return Errf("Des collègues doivent encore te rembourser : attends la clôture de la commande avant de supprimer ton compte.")
	}
	if isAdmin && activeAdmins <= 1 {
		return Errf("Tu es le dernier compte administrateur : nomme d'abord un autre admin.")
	}
	return nil
}

package domain

import (
	"slices"
	"strings"
	"testing"
	"time"
)

var bt0 = time.Date(2026, 10, 10, 0, 15, 0, 0, time.UTC)

func bo(key string, hoursAgo int) BackupObject {
	return BackupObject{Key: key, Size: 100, Modified: bt0.Add(-time.Duration(hoursAgo) * time.Hour)}
}

func TestIsBackupArchive(t *testing.T) {
	cases := map[string]bool{
		"@auto_pb_backup_occ_20261010001500.zip": true,
		"pb_backup_x.ZIP":                        true,
		"":                                       false,
		"notes.txt":                              false,
		"dir/backup.zip":                         false,
		"backup.zip.part":                        false,
	}
	for k, want := range cases {
		if got := IsBackupArchive(k); got != want {
			t.Errorf("%q: got %v want %v", k, got, want)
		}
	}
}

func TestLatestAndSortBackups(t *testing.T) {
	tests := []struct {
		name string
		objs []BackupObject
		want string
		ok   bool
	}{
		{"vide", nil, "", false},
		{"sans archive", []BackupObject{bo("x.txt", 0)}, "", false},
		{"plus récente", []BackupObject{bo("a.zip", 48), bo("b.zip", 1), bo("c.zip", 24), bo("d.txt", 0)}, "b.zip", true},
		{"égalité : nom", []BackupObject{bo("@auto_1.zip", 2), bo("@auto_2.zip", 2)}, "@auto_2.zip", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := LatestBackup(tc.objs)
			if ok != tc.ok || got.Key != tc.want {
				t.Fatalf("got %q %v", got.Key, ok)
			}
		})
	}
	sorted := SortBackups([]BackupObject{bo("a.zip", 48), bo("b.zip", 1), bo("c.zip", 24)})
	var keys []string
	for _, o := range sorted {
		keys = append(keys, o.Key)
	}
	if !slices.Equal(keys, []string{"b.zip", "c.zip", "a.zip"}) {
		t.Fatal(keys)
	}
}

func TestBackupsToPrune(t *testing.T) {
	objs := []BackupObject{
		bo("@auto_pb_backup_occ_1.zip", 72), bo("@auto_pb_backup_occ_2.zip", 48),
		bo("@auto_pb_backup_occ_3.zip", 24), bo("@auto_pb_backup_occ_4.zip", 0),
		bo("pb_backup_manuel.zip", 100), // made in /_/: never pruned
	}
	tests := []struct {
		name string
		keep int
		want []string
	}{
		{"sans limite", 0, nil},
		{"négatif", -1, nil},
		{"garde tout", 4, nil},
		{"garde plus", 10, nil},
		{"garde 2", 2, []string{"@auto_pb_backup_occ_2.zip", "@auto_pb_backup_occ_1.zip"}},
		{"garde 1", 1, []string{"@auto_pb_backup_occ_3.zip", "@auto_pb_backup_occ_2.zip", "@auto_pb_backup_occ_1.zip"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := BackupsToPrune(objs, "@auto_pb_backup_", tc.keep); !slices.Equal(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestCheckBackupHealth(t *testing.T) {
	recent := bt0.Add(-2 * time.Hour)
	old := bt0.Add(-37 * time.Hour)
	edge := bt0.Add(-36 * time.Hour)
	startedNow := bt0.Add(-time.Hour)
	startedLong := bt0.Add(-40 * time.Hour)
	tests := []struct {
		name    string
		enabled bool
		latest  *time.Time
		since   time.Time
		want    BackupHealth
	}{
		{"désactivé", false, &recent, startedLong, BackupDisabled},
		{"récente", true, &recent, startedLong, BackupOK},
		{"36 h pile", true, &edge, startedLong, BackupOK},
		{"trop vieille", true, &old, startedNow, BackupStale},
		{"aucune, démarrage récent", true, nil, startedNow, BackupPending},
		{"aucune, démarré depuis longtemps", true, nil, startedLong, BackupStale},
		{"zéro = aucune", true, &time.Time{}, startedNow, BackupPending},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := CheckBackupHealth(tc.enabled, tc.latest, tc.since, bt0, BackupMaxAge); got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}

func TestShouldSendBackupAlert(t *testing.T) {
	tests := []struct {
		name   string
		health BackupHealth
		last   time.Time
		want   bool
	}{
		{"ok", BackupOK, time.Time{}, false},
		{"en attente", BackupPending, time.Time{}, false},
		{"jamais alerté", BackupStale, time.Time{}, true},
		{"alerté il y a 3 h", BackupStale, bt0.Add(-3 * time.Hour), false},
		{"alerté il y a 24 h", BackupStale, bt0.Add(-24 * time.Hour), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShouldSendBackupAlert(tc.health, tc.last, bt0); got != tc.want {
				t.Fatalf("got %v", got)
			}
		})
	}
}

func TestCheckBackupCounts(t *testing.T) {
	tests := []struct {
		name      string
		c         TableCount
		errs      int
		warns     int
		errSubstr string
	}{
		{"identique", TableCount{Name: "users", Label: "utilisateurs", Backup: 42, Live: 42}, 0, 0, ""},
		{"un peu moins", TableCount{Name: "parties", Label: "commandes", Backup: 28, Live: 30}, 0, 0, ""},
		{"base vide", TableCount{Name: "users", Backup: 0, Live: 0}, 0, 0, ""},
		{"petite base, sauvegarde vide", TableCount{Name: "users", Label: "utilisateurs", Backup: 0, Live: 3}, 1, 0, "aucun enregistrement"},
		{"petite base, moitié tolérée", TableCount{Name: "parties", Backup: 2, Live: 10}, 0, 0, ""},
		{"moins de la moitié", TableCount{Name: "restaurants", Label: "restaurants", Backup: 9, Live: 61}, 1, 0, "moins de la moitié"},
		{"moitié pile", TableCount{Name: "restaurants", Backup: 30, Live: 60}, 0, 0, ""},
		{"beaucoup plus", TableCount{Name: "parties", Label: "commandes", Backup: 100, Live: 20}, 0, 1, ""},
		{"un peu plus", TableCount{Name: "parties", Backup: 25, Live: 20}, 0, 0, ""},
		{"comptage impossible", TableCount{Name: "users", Backup: -1, Live: 3}, 1, 0, "Comptage impossible"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs, warns := CheckBackupCounts([]TableCount{tc.c})
			if len(errs) != tc.errs || len(warns) != tc.warns {
				t.Fatalf("errs %v warns %v", errs, warns)
			}
			if tc.errSubstr != "" && !strings.Contains(errs[0], tc.errSubstr) {
				t.Fatalf("message: %v", errs)
			}
		})
	}
}

func TestMissingTablesAndPragma(t *testing.T) {
	if got := MissingTables([]string{"users", "parties", "restaurants"}, []string{"parties", "users", "_params"}); !slices.Equal(got, []string{"restaurants"}) {
		t.Fatal(got)
	}
	if got := MissingTables([]string{"users"}, []string{"users"}); got != nil {
		t.Fatal(got)
	}
	pragmas := []struct {
		rows []string
		want bool
	}{
		{[]string{"ok"}, true},
		{[]string{" OK "}, true},
		{nil, false},
		{[]string{"*** in database main ***", "Page 3 is never used"}, false},
		{[]string{"row 1 missing from index"}, false},
	}
	for _, p := range pragmas {
		if got := PragmaOK(p.rows); got != p.want {
			t.Errorf("%v: got %v", p.rows, got)
		}
	}
}

// Command menusync retrieves restaurant + menu feeds (Deliveroo, weloveat,
// Takeaway mini-sites, schema.org JSON-LD) into the RestaurantImport JSON
// format, merges feeds, and optionally imports them into OCC DELIVERIES.
//
//	menusync -provider deliveroo -city mons -limit 5 -out mons-deliveroo.json
//	menusync -provider takeaway-site -urls sites.txt -out mons-takeaway-sites.json
//	menusync merge -in a.json,b.json -out mons-merged.json
//	menusync -provider deliveroo -import https://eat.fs0ciety.org -token <jeton> -dry-run
//
// See docs/DEPLOYMENT.md « Gérer les restaurants ».
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/menusync"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "merge" {
		os.Exit(mergeCmd(os.Args[2:]))
	}
	os.Exit(fetchCmd(os.Args[1:]))
}

func logf(format string, args ...any) { fmt.Fprintf(os.Stderr, format+"\n", args...) }

func fetchCmd(args []string) int {
	fs := flag.NewFlagSet("menusync", flag.ContinueOnError)
	provider := fs.String("provider", "", "source : deliveroo | weloveat | takeaway-site | jsonld")
	source := fs.String("source", "", "alias de -provider")
	city := fs.String("city", "mons", "ville (centre de recherche)")
	radius := fs.Float64("radius", 8, "rayon max en km autour du centre de la ville (0 = illimité)")
	limit := fs.Int("limit", 0, "nombre max de restaurants (0 = tous)")
	out := fs.String("out", "", "fichier JSON de sortie (défaut : <ville>-<source>.json)")
	cache := fs.String("cache", ".menusync-cache", "dossier de cache disque (\"\" = désactivé)")
	delay := fs.Duration("delay", menusync.MinDelay, "pause entre deux requêtes (minimum 2.5s)")
	details := fs.Bool("options", false, "weloveat : lire aussi chaque produit pour ses suppléments (1 requête de plus par plat : long)")
	oneURL := fs.String("url", "", "takeaway-site / jsonld : une URL de site")
	urlsFile := fs.String("urls", "", "takeaway-site / jsonld : fichier d'URL (une par ligne, # = commentaire)")
	importURL := fs.String("import", "", "importer dans cette instance (ex. https://eat.fs0ciety.org)")
	token := fs.String("token", "", "jeton superutilisateur PocketBase (ou variable OCC_IMPORT_TOKEN)")
	dryRun := fs.Bool("dry-run", false, "avec -import : afficher sans envoyer")
	in := fs.String("in", "", "avec -import : importer ce fichier JSON au lieu de récupérer un flux")
	fs.BoolVar(&pretty, "pretty", false, "JSON indenté (défaut : un restaurant par ligne)")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage : menusync -provider deliveroo|weloveat|takeaway-site|jsonld [options]\n        menusync merge -in a.json,b.json -out fusion.json [-priority …]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	src := strings.ToLower(strings.TrimSpace(*provider + *source))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var list []menusync.Restaurant
	if *in != "" {
		var err error
		if list, err = readFile(*in); err != nil {
			logf("erreur : %v", err)
			return 1
		}
	} else {
		center, ok := menusync.Cities[strings.ToLower(*city)]
		if !ok {
			logf("erreur : ville %q inconnue (connues : mons)", *city)
			return 2
		}
		urls, err := collectURLs(*oneURL, *urlsFile)
		if err != nil {
			logf("erreur : %v", err)
			return 2
		}
		f := menusync.NewFetcher(*cache)
		f.Delay = *delay
		f.Logf = logf
		opts := menusync.Options{
			City: strings.ToLower(*city), Center: center, RadiusKm: *radius, Limit: *limit,
			Details: *details, URLs: urls, Today: menusync.Today(), Logf: logf,
		}
		start := time.Now()
		list, err = menusync.Run(ctx, f, src, opts)
		blocked := menusync.IsBlocked(err)
		if err != nil && !blocked && len(list) == 0 {
			logf("erreur : %v", err)
			return 1
		}
		path := *out
		if path == "" {
			path = fmt.Sprintf("%s-%s.json", opts.City, src)
		}
		if werr := writeFile(path, list); werr != nil {
			logf("erreur : %v", werr)
			return 1
		}
		report(list)
		logf("Requêtes réseau : %d, depuis le cache : %d, durée : %s", f.Network, f.Cached, time.Since(start).Round(time.Second))
		logf("Écrit : %s", path)
		if err != nil {
			logf("ARRÊT : %v", err)
			logf("Le fichier contient les restaurants récupérés avant l'arrêt.")
			return 3
		}
	}

	if *importURL != "" {
		t := *token
		if t == "" {
			t = os.Getenv("OCC_IMPORT_TOKEN")
		}
		if t == "" && !*dryRun {
			logf("erreur : -token (ou OCC_IMPORT_TOKEN) requis pour -import")
			return 2
		}
		if err := importAll(ctx, *importURL, t, list, *dryRun); err != nil {
			logf("erreur d'import : %v", err)
			return 1
		}
	}
	return 0
}

func report(list []menusync.Restaurant) {
	items, withOpts, restWithOpts := 0, 0, 0
	for _, r := range list {
		items += r.ItemCount()
		withOpts += r.ItemsWithOptions()
		if r.ItemsWithOptions() > 0 {
			restWithOpts++
		}
	}
	logf("Bilan : %d restaurants, %d plats (%d avec options, %d sans), %d restaurants avec options",
		len(list), items, withOpts, items-withOpts, restWithOpts)
}

func collectURLs(one, file string) ([]string, error) {
	var urls []string
	if one != "" {
		urls = append(urls, strings.TrimSpace(one))
	}
	if file != "" {
		fh, err := os.Open(file)
		if err != nil {
			return nil, err
		}
		defer fh.Close()
		sc := bufio.NewScanner(fh)
		for sc.Scan() {
			l := strings.TrimSpace(sc.Text())
			if l != "" && !strings.HasPrefix(l, "#") {
				urls = append(urls, l)
			}
		}
		if err := sc.Err(); err != nil {
			return nil, err
		}
	}
	return urls, nil
}

func readFile(path string) ([]menusync.Restaurant, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var list []menusync.Restaurant
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, fmt.Errorf("%s : %w", path, err)
	}
	return list, nil
}

// pretty switches the JSON output to indented JSON (default: one compact
// restaurant per line, readable diffs at a third of the size).
var pretty bool

func writeFile(path string, list []menusync.Restaurant) error {
	if list == nil {
		list = []menusync.Restaurant{}
	}
	var buf bytes.Buffer
	if pretty {
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(list); err != nil {
			return err
		}
	} else {
		buf.WriteString("[")
		for i, r := range list {
			var line bytes.Buffer
			enc := json.NewEncoder(&line)
			enc.SetEscapeHTML(false)
			if err := enc.Encode(r); err != nil {
				return err
			}
			if i > 0 {
				buf.WriteString(",")
			}
			buf.WriteString("\n")
			buf.Write(bytes.TrimRight(line.Bytes(), "\n"))
		}
		buf.WriteString("\n]\n")
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func importAll(ctx context.Context, base, token string, list []menusync.Restaurant, dry bool) error {
	endpoint := strings.TrimSuffix(base, "/") + "/api/occ/admin/import"
	client := &http.Client{Timeout: 60 * time.Second}
	for _, r := range list {
		if dry {
			logf("[dry-run] POST %s ← %s (%s) : %d plats", endpoint, r.Name, r.Slug, r.ItemCount())
			continue
		}
		body, err := json.Marshal(r)
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", token)
		req.Header.Set("User-Agent", menusync.UserAgent)
		res, err := client.Do(req)
		if err != nil {
			return err
		}
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return fmt.Errorf("%s : HTTP %d %s", r.Slug, res.StatusCode, strings.TrimSpace(string(msg)))
		}
		logf("importé : %s (%s) %s", r.Name, r.Slug, strings.TrimSpace(string(msg)))
	}
	return nil
}

func mergeCmd(args []string) int {
	fs := flag.NewFlagSet("menusync merge", flag.ContinueOnError)
	in := fs.String("in", "", "fichiers à fusionner, séparés par des virgules ; préfixe facultatif source=fichier")
	out := fs.String("out", "merged.json", "fichier JSON de sortie")
	fs.BoolVar(&pretty, "pretty", false, "JSON indenté (défaut : un restaurant par ligne)")
	prio := fs.String("priority", strings.Join(menusync.DefaultPriority, ","), "ordre de préférence des sources pour le menu")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *in == "" {
		logf("erreur : -in requis")
		return 2
	}
	var all []menusync.Restaurant
	for _, spec := range strings.Split(*in, ",") {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}
		src, path, hasSrc := strings.Cut(spec, "=")
		if !hasSrc {
			path, src = spec, sourceFromName(spec)
		}
		list, err := readFile(path)
		if err != nil {
			logf("erreur : %v", err)
			return 1
		}
		for i := range list {
			if hasSrc || list[i].Source == "" {
				list[i].Source = src
			}
		}
		logf("%s : %d restaurants (source %s)", path, len(list), src)
		all = append(all, list...)
	}
	var priority []string
	for _, p := range strings.Split(*prio, ",") {
		if p = strings.TrimSpace(p); p != "" {
			priority = append(priority, p)
		}
	}
	merged, stats := menusync.Merge(all, priority)
	for i := range merged {
		merged[i].Normalize()
		if err := merged[i].RestaurantImport.Validate(); err != nil {
			logf("attention : %s (%s) ne passera pas l'import : %v", merged[i].Name, merged[i].Slug, err)
		}
	}
	if err := writeFile(*out, merged); err != nil {
		logf("erreur : %v", err)
		return 1
	}
	for _, g := range stats.Merged {
		logf("fusionné %s ← %s", g.Slug, strings.Join(g.Sources, " | "))
	}
	logf("Fusion : %d fiches en entrée, %d restaurants en sortie, %d doublons fusionnés", stats.In, stats.Out, stats.Duplicates())
	report(merged)
	logf("Écrit : %s", *out)
	return 0
}

func sourceFromName(path string) string {
	base := strings.ToLower(filepath.Base(path))
	for _, s := range []string{menusync.SourceTakeawaySite, "takeaway_site", menusync.SourceDeliveroo, menusync.SourceWeloveat, menusync.SourceJSONLD} {
		if strings.Contains(base, s) {
			if s == "takeaway_site" {
				return menusync.SourceTakeawaySite
			}
			return s
		}
	}
	return menusync.SourceExisting
}

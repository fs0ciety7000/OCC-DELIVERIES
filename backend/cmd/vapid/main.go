// Command vapid prints a new VAPID key pair for Web Push, ready to paste in
// the Coolify environment (see docs/DEPLOYMENT.md):
//
//	go run ./cmd/vapid
package main

import (
	"fmt"
	"os"

	"github.com/fs0ciety7000/occ-deliveries/backend/internal/notify"
)

func main() {
	keys, err := notify.GenerateKeys()
	if err != nil {
		fmt.Fprintln(os.Stderr, "génération impossible :", err)
		os.Exit(1)
	}
	fmt.Println("# Clés VAPID (Web Push) — la clé privée est un secret : ne la commitez jamais.")
	fmt.Println("OCC_VAPID_PUBLIC_KEY=" + keys.Public)
	fmt.Println("OCC_VAPID_PRIVATE_KEY=" + keys.Private)
	fmt.Println("OCC_VAPID_SUBJECT=mailto:noreply@example.org")
}

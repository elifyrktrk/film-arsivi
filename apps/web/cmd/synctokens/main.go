// Command synctokens copies the shared design tokens (packages/tokens/dist/tokens.css)
// into the web app's embedded static files. Run via `go generate ./...` from apps/web.
// The source is never modified; tokens_test.go fails when the copy is stale.
package main

import (
	"log"
	"os"
	"path/filepath"
)

func main() {
	src := filepath.Join("..", "..", "packages", "tokens", "dist", "tokens.css")
	dst := filepath.Join("internal", "web", "static", "tokens.css")
	if len(os.Args) == 3 {
		src, dst = os.Args[1], os.Args[2]
	}
	b, err := os.ReadFile(src)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("copied %s -> %s (%d bytes)", src, dst, len(b))
}

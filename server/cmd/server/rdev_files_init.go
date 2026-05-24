// rdev_files_init.go wires the rdev/files file browser into the server's
// extension route system. VCS sources (Gitea/GitHub) are fully supported;
// runtime daemon sources require a follow-up to export Hub.SendFrame.
package main

import (
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/extension"
	"github.com/zinohome/RDev/rdev/files"
	"github.com/zinohome/RDev/rdev/gitea"
	"github.com/zinohome/RDev/rdev/vcs"
)

func init() {
	vcsReg := vcs.NewRegistry()

	// Register Gitea if RDEV_GITEA_URL + RDEV_GITEA_TOKEN are set.
	if url, tok := strings.TrimSpace(os.Getenv("RDEV_GITEA_URL")),
		strings.TrimSpace(os.Getenv("RDEV_GITEA_TOKEN")); url != "" && tok != "" {
		vcsReg.Register(gitea.New(url, tok))
	}

	extension.RegisterExtensionRoutes(func(r chi.Router) {
		files.Register(r, files.Config{
			VCSRegistry: vcsReg,
			// Hub: nil → runtime sources return 503 until Hub.SendFrame is exported.
		})
	})
}

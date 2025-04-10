package manager

import (
	"os"

	"github.com/rs/zerolog"

	"github.com/omnicate/flx/loader/controller"
)

type Option func(*Loader)

func WithRepoCachePath(path string) Option {
	return func(l *Loader) {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			_ = os.MkdirAll(path, 0755)
		}
		l.repoCachePath = path
	}
}

func WithLogger(logger zerolog.Logger) Option {
	return func(l *Loader) {
		l.logger = logger
	}
}

func WithLocalRepoRef(lgr ...*controller.GitLocalReplace) Option {
	return func(l *Loader) {
		l.repoReplace = append(l.repoReplace, lgr...)
	}
}

func WithGitForceHTTPS(forceHTTPS bool) Option {
	return func(l *Loader) {
		l.gitViaHTTPS = forceHTTPS
	}
}

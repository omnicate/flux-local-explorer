package loader

import (
	"os"

	"github.com/rs/zerolog"
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

type LocalGitRepository struct {
	Remote string
	Path   string

	Commit string
	Branch string
	Tag    string
}

func (l *LocalGitRepository) Ref() string {
	return orDefault(orDefault(l.Commit, l.Branch), l.Tag)
}

func WithLocalRepoRef(lgr ...*LocalGitRepository) Option {
	return func(l *Loader) {
		l.repoReplace = append(l.repoReplace, lgr...)
	}
}

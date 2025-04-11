package git

import (
	ctrl "github.com/omnicate/flx/loader/controller"
)

type Options struct {
	CachePath string
	UseHTTPS  bool
	Local     []*LocalReplace
}

type LocalReplace struct {
	Remote string
	Path   string

	Commit string
	Branch string
	Tag    string
}

func (l *LocalReplace) Ref() string {
	return ctrl.Any(l.Commit, l.Branch, l.Tag)
}

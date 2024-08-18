package repo

import "path"

type Pkg struct {
	Path, Version string
}

func (pkg Pkg) Name() string {
	return path.Base(pkg.Path)
}

func (pkg Pkg) Latest() Pkg {
	return Pkg{pkg.Path, "latest"}
}

func (pkg Pkg) String() string {
	return pkg.Path + "@" + pkg.Version
}

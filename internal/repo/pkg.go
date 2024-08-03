package repo

type Pkg struct {
	Path, Version string
}

func (pkg Pkg) Latest() Pkg {
	return Pkg{pkg.Path, "latest"}
}

func (pkg Pkg) String() string {
	return pkg.Path + "@" + pkg.Version
}

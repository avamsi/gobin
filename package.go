package main

import "path"

type pkge struct {
	path, version string
}

func (pkg pkge) name() string {
	return path.Base(pkg.path)
}

func (pkg pkge) String() string {
	return pkg.path + "@" + pkg.version
}

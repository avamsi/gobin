package main

import "path"

type pkge struct {
	path, version string
}

func (pkg pkge) String() string {
	return pkg.path + "@" + pkg.version
}

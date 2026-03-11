package version

import "runtime/debug"

func Get() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "<unknown>"
	}
	if info.Main.Version == "" {
		return "(devel)"
	}
	return info.Main.Version
}

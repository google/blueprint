package bootstrap

import (
	"blueprint"
)

var (
	// These variables are the only only configuration needed by the boostrap
	// modules.  They are always set to the variable name enclosed in "@@" so
	// that their values can be easily replaced in the generated Ninja file.
	SrcDir            = blueprint.StaticVariable("SrcDir", "@@SrcDir@@")
	GoRoot            = blueprint.StaticVariable("GoRoot", "@@GoRoot@@")
	GoOS              = blueprint.StaticVariable("GoOS", "@@GoOS@@")
	GoArch            = blueprint.StaticVariable("GoArch", "@@GoArch@@")
	GoChar            = blueprint.StaticVariable("GoChar", "@@GoChar@@")
	Bootstrap         = blueprint.StaticVariable("Bootstrap", "@@Bootstrap@@")
	BootstrapManifest = blueprint.StaticVariable("BootstrapManifest",
		"@@BootstrapManifest@@")

	goToolDir = blueprint.StaticVariable("goToolDir",
		"$GoRoot/pkg/tool/${GoOS}_$GoArch")
)

type Config interface {
	// GeneratingBootstrapper should return true if this build invocation is
	// creating a build.ninja.in file to be used in a build bootstrapping
	// sequence.
	GeneratingBootstrapper() bool
}

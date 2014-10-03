package bootstrap

var (
	// These variables are the only configuration needed by the boostrap
	// modules.  They are always set to the variable name enclosed in "@@" so
	// that their values can be easily replaced in the generated Ninja file.
	SrcDir            = pctx.StaticVariable("SrcDir", "@@SrcDir@@")
	GoRoot            = pctx.StaticVariable("GoRoot", "@@GoRoot@@")
	GoOS              = pctx.StaticVariable("GoOS", "@@GoOS@@")
	GoArch            = pctx.StaticVariable("GoArch", "@@GoArch@@")
	GoChar            = pctx.StaticVariable("GoChar", "@@GoChar@@")
	Bootstrap         = pctx.StaticVariable("Bootstrap", "@@Bootstrap@@")
	BootstrapManifest = pctx.StaticVariable("BootstrapManifest",
		"@@BootstrapManifest@@")

	goToolDir = pctx.StaticVariable("goToolDir",
		"$GoRoot/pkg/tool/${GoOS}_$GoArch")
)

type Config interface {
	// GeneratingBootstrapper should return true if this build invocation is
	// creating a build.ninja.in file to be used in a build bootstrapping
	// sequence.
	GeneratingBootstrapper() bool
}

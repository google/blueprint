package bootstrap

var (
	// These variables are the only configuration needed by the boostrap
	// modules.  They are always set to the variable name enclosed in "@@" so
	// that their values can be easily replaced in the generated Ninja file.
	srcDir            = pctx.StaticVariable("srcDir", "@@SrcDir@@")
	goRoot            = pctx.StaticVariable("goRoot", "@@GoRoot@@")
	goOS              = pctx.StaticVariable("goOS", "@@GoOS@@")
	goArch            = pctx.StaticVariable("goArch", "@@GoArch@@")
	goChar            = pctx.StaticVariable("goChar", "@@GoChar@@")
	bootstrapCmd      = pctx.StaticVariable("bootstrapCmd", "@@Bootstrap@@")
	bootstrapManifest = pctx.StaticVariable("bootstrapManifest",
		"@@BootstrapManifest@@")

	goToolDir = pctx.StaticVariable("goToolDir",
		"$goRoot/pkg/tool/${goOS}_$goArch")
)

type Config interface {
	// GeneratingBootstrapper should return true if this build invocation is
	// creating a build.ninja.in file to be used in a build bootstrapping
	// sequence.
	GeneratingBootstrapper() bool
}

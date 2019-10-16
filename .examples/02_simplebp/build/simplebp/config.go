package simplebp

// A config holds all of the data that's unique to this build instance. For
// example, source and output directories will depend on the results of
// bootstrapping, and so they are stored here for use in modules.
type config struct {
	srcDir   string
	buildDir string
}

func NewConfig(srcDir string, buildDir string) interface{} {
	return &config{srcDir, buildDir}
}

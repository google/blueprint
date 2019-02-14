package bootstrap

import (
	"bytes"
	"fmt"
	"html/template"
	"io/ioutil"
	"path/filepath"
	"reflect"

	"github.com/google/blueprint"
	"github.com/google/blueprint/bootstrap/bpdoc"
	"github.com/google/blueprint/pathtools"
)

// ModuleTypeDocs returns a list of bpdoc.ModuleType objects that contain information relevant
// to generating documentation for module types supported by the primary builder.
func ModuleTypeDocs(ctx *blueprint.Context, factories map[string]reflect.Value) ([]*bpdoc.Package, error) {
	// Find the module that's marked as the "primary builder", which means it's
	// creating the binary that we'll use to generate the non-bootstrap
	// build.ninja file.
	var primaryBuilders []*goBinary
	var minibp *goBinary
	ctx.VisitAllModulesIf(isBootstrapBinaryModule,
		func(module blueprint.Module) {
			binaryModule := module.(*goBinary)
			if binaryModule.properties.PrimaryBuilder {
				primaryBuilders = append(primaryBuilders, binaryModule)
			}
			if ctx.ModuleName(binaryModule) == "minibp" {
				minibp = binaryModule
			}
		})

	if minibp == nil {
		panic("missing minibp")
	}

	var primaryBuilder *goBinary
	switch len(primaryBuilders) {
	case 0:
		// If there's no primary builder module then that means we'll use minibp
		// as the primary builder.
		primaryBuilder = minibp

	case 1:
		primaryBuilder = primaryBuilders[0]

	default:
		return nil, fmt.Errorf("multiple primary builder modules present")
	}

	pkgFiles := make(map[string][]string)
	ctx.VisitDepsDepthFirst(primaryBuilder, func(module blueprint.Module) {
		switch m := module.(type) {
		case (*goPackage):
			pkgFiles[m.properties.PkgPath] = pathtools.PrefixPaths(m.properties.Srcs,
				filepath.Join(SrcDir, ctx.ModuleDir(m)))
		default:
			panic(fmt.Errorf("unknown dependency type %T", module))
		}
	})

	mergedFactories := make(map[string]reflect.Value)
	for moduleType, factory := range factories {
		mergedFactories[moduleType] = factory
	}

	for moduleType, factory := range ctx.ModuleTypeFactories() {
		if _, exists := mergedFactories[moduleType]; !exists {
			mergedFactories[moduleType] = reflect.ValueOf(factory)
		}
	}

	return bpdoc.AllPackages(pkgFiles, mergedFactories, ctx.ModuleTypePropertyStructs())
}

func writeDocs(ctx *blueprint.Context, filename string) error {
	moduleTypeList, err := ModuleTypeDocs(ctx, nil)
	if err != nil {
		return err
	}

	buf := &bytes.Buffer{}

	unique := 0

	tmpl, err := template.New("file").Funcs(map[string]interface{}{
		"unique": func() int {
			unique++
			return unique
		}}).Parse(fileTemplate)
	if err != nil {
		return err
	}

	err = tmpl.Execute(buf, moduleTypeList)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(filename, buf.Bytes(), 0666)
	if err != nil {
		return err
	}

	return nil
}

const (
	fileTemplate = `
<html>
<head>
<title>Build Docs</title>
<link rel="stylesheet" href="https://maxcdn.bootstrapcdn.com/bootstrap/3.3.5/css/bootstrap.min.css">
<script src="https://ajax.googleapis.com/ajax/libs/jquery/2.1.4/jquery.min.js"></script>
<script src="https://maxcdn.bootstrapcdn.com/bootstrap/3.3.5/js/bootstrap.min.js"></script>
</head>
<body>
<h1>Build Docs</h1>
<div class="panel-group" id="accordion" role="tablist" aria-multiselectable="true">
  {{range .}}
    <p>{{.Text}}</p>
    {{range .ModuleTypes}}
      {{ $collapseIndex := unique }}
      <div class="panel panel-default">
        <div class="panel-heading" role="tab" id="heading{{$collapseIndex}}">
          <h2 class="panel-title">
            <a class="collapsed" role="button" data-toggle="collapse" data-parent="#accordion" href="#collapse{{$collapseIndex}}" aria-expanded="false" aria-controls="collapse{{$collapseIndex}}">
               {{.Name}}
            </a>
          </h2>
        </div>
      </div>
      <div id="collapse{{$collapseIndex}}" class="panel-collapse collapse" role="tabpanel" aria-labelledby="heading{{$collapseIndex}}">
        <div class="panel-body">
          <p>{{.Text}}</p>
          {{range .PropertyStructs}}
            <p>{{.Text}}</p>
            {{template "properties" .Properties}}
          {{end}}
        </div>
      </div>
    {{end}}
  {{end}}
</div>
</body>
</html>

{{define "properties"}}
  <div class="panel-group" id="accordion" role="tablist" aria-multiselectable="true">
    {{range .}}
      {{$collapseIndex := unique}}
      {{if .Properties}}
        <div class="panel panel-default">
          <div class="panel-heading" role="tab" id="heading{{$collapseIndex}}">
            <h4 class="panel-title">
              <a class="collapsed" role="button" data-toggle="collapse" data-parent="#accordion" href="#collapse{{$collapseIndex}}" aria-expanded="false" aria-controls="collapse{{$collapseIndex}}">
                 {{.Name}}{{range .OtherNames}}, {{.}}{{end}}
              </a>
            </h4>
          </div>
        </div>
        <div id="collapse{{$collapseIndex}}" class="panel-collapse collapse" role="tabpanel" aria-labelledby="heading{{$collapseIndex}}">
          <div class="panel-body">
            <p>{{.Text}}</p>
            {{range .OtherTexts}}<p>{{.}}</p>{{end}}
            {{template "properties" .Properties}}
          </div>
        </div>
      {{else}}
        <div>
          <h4>{{.Name}}{{range .OtherNames}}, {{.}}{{end}}</h4>
          <p>{{.Text}}</p>
          {{range .OtherTexts}}<p>{{.}}</p>{{end}}
          <p><i>Type: {{.Type}}</i></p>
          {{if .Default}}<p><i>Default: {{.Default}}</i></p>{{end}}
        </div>
      {{end}}
    {{end}}
  </div>
{{end}}
`
)

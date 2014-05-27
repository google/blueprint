package blueprint

func packageNamespacePrefix(packageName string) string {
	return "g." + packageName + "."
}

func moduleNamespacePrefix(moduleName string) string {
	return "m." + moduleName + "."
}

func singletonNamespacePrefix(singletonName string) string {
	return "s." + singletonName + "."
}

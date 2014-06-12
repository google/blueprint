package blueprint

// A liveTracker tracks the values of live variables, rules, and pools.  An
// entity is made "live" when it is referenced directly or indirectly by a build
// definition.  When an entity is made live its value is computed based on the
// configuration.
type liveTracker struct {
	config interface{} // Used to evaluate variable, rule, and pool values.

	variables map[Variable]*ninjaString
	pools     map[Pool]*poolDef
	rules     map[Rule]*ruleDef
}

func newLiveTracker(config interface{}) *liveTracker {
	return &liveTracker{
		config:    config,
		variables: make(map[Variable]*ninjaString),
		pools:     make(map[Pool]*poolDef),
		rules:     make(map[Rule]*ruleDef),
	}
}

func (l *liveTracker) AddBuildDefDeps(def *buildDef) error {
	err := l.addRule(def.Rule)
	if err != nil {
		return err
	}

	err = l.addNinjaStringListDeps(def.Outputs)
	if err != nil {
		return err
	}

	err = l.addNinjaStringListDeps(def.Inputs)
	if err != nil {
		return err
	}

	err = l.addNinjaStringListDeps(def.Implicits)
	if err != nil {
		return err
	}

	err = l.addNinjaStringListDeps(def.OrderOnly)
	if err != nil {
		return err
	}

	for _, value := range def.Args {
		err = l.addNinjaStringDeps(value)
		if err != nil {
			return err
		}
	}

	return nil
}

func (l *liveTracker) addRule(r Rule) error {
	_, ok := l.rules[r]
	if !ok {
		def, err := r.def(l.config)
		if err == errRuleIsBuiltin {
			// No need to do anything for built-in rules.
			return nil
		}
		if err != nil {
			return err
		}

		if def.Pool != nil {
			err = l.addPool(def.Pool)
			if err != nil {
				return err
			}
		}

		for _, value := range def.Variables {
			err = l.addNinjaStringDeps(value)
			if err != nil {
				return err
			}
		}

		l.rules[r] = def
	}

	return nil
}

func (l *liveTracker) addPool(p Pool) error {
	_, ok := l.pools[p]
	if !ok {
		def, err := p.def(l.config)
		if err != nil {
			return err
		}

		l.pools[p] = def
	}

	return nil
}

func (l *liveTracker) addVariable(v Variable) error {
	_, ok := l.variables[v]
	if !ok {
		value, err := v.value(l.config)
		if err == errVariableIsArg {
			// This variable is a placeholder for an argument that can be passed
			// to a rule.  It has no value and thus doesn't reference any other
			// variables.
			return nil
		}
		if err != nil {
			return err
		}

		l.variables[v] = value

		err = l.addNinjaStringDeps(value)
		if err != nil {
			return err
		}
	}

	return nil
}

func (l *liveTracker) addNinjaStringListDeps(list []*ninjaString) error {
	for _, str := range list {
		err := l.addNinjaStringDeps(str)
		if err != nil {
			return err
		}
	}
	return nil
}

func (l *liveTracker) addNinjaStringDeps(str *ninjaString) error {
	for _, v := range str.variables {
		err := l.addVariable(v)
		if err != nil {
			return err
		}
	}
	return nil
}

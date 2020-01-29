// Copyright 2014 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package blueprint

import "sync"

// A liveTracker tracks the values of live variables, rules, and pools.  An
// entity is made "live" when it is referenced directly or indirectly by a build
// definition.  When an entity is made live its value is computed based on the
// configuration.
type liveTracker struct {
	sync.Mutex
	config interface{} // Used to evaluate variable, rule, and pool values.

	variables map[Variable]ninjaString
	pools     map[Pool]*poolDef
	rules     map[Rule]*ruleDef
}

func newLiveTracker(config interface{}) *liveTracker {
	return &liveTracker{
		config:    config,
		variables: make(map[Variable]ninjaString),
		pools:     make(map[Pool]*poolDef),
		rules:     make(map[Rule]*ruleDef),
	}
}

func (l *liveTracker) AddBuildDefDeps(def *buildDef) error {
	l.Lock()
	defer l.Unlock()

	ruleDef, err := l.addRule(def.Rule)
	if err != nil {
		return err
	}
	def.RuleDef = ruleDef

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

	for _, value := range def.Variables {
		err = l.addNinjaStringDeps(value)
		if err != nil {
			return err
		}
	}

	for _, value := range def.Args {
		err = l.addNinjaStringDeps(value)
		if err != nil {
			return err
		}
	}

	return nil
}

func (l *liveTracker) addRule(r Rule) (def *ruleDef, err error) {
	def, ok := l.rules[r]
	if !ok {
		def, err = r.def(l.config)
		if err == errRuleIsBuiltin {
			// No need to do anything for built-in rules.
			return nil, nil
		}
		if err != nil {
			return nil, err
		}

		if def.Pool != nil {
			err = l.addPool(def.Pool)
			if err != nil {
				return nil, err
			}
		}

		err = l.addNinjaStringListDeps(def.CommandDeps)
		if err != nil {
			return nil, err
		}

		err = l.addNinjaStringListDeps(def.CommandOrderOnly)
		if err != nil {
			return nil, err
		}

		for _, value := range def.Variables {
			err = l.addNinjaStringDeps(value)
			if err != nil {
				return nil, err
			}
		}

		l.rules[r] = def
	}

	return
}

func (l *liveTracker) addPool(p Pool) error {
	_, ok := l.pools[p]
	if !ok {
		def, err := p.def(l.config)
		if err == errPoolIsBuiltin {
			// No need to do anything for built-in rules.
			return nil
		}
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

func (l *liveTracker) addNinjaStringListDeps(list []ninjaString) error {
	for _, str := range list {
		err := l.addNinjaStringDeps(str)
		if err != nil {
			return err
		}
	}
	return nil
}

func (l *liveTracker) addNinjaStringDeps(str ninjaString) error {
	for _, v := range str.Variables() {
		err := l.addVariable(v)
		if err != nil {
			return err
		}
	}
	return nil
}

func (l *liveTracker) RemoveVariableIfLive(v Variable) bool {
	l.Lock()
	defer l.Unlock()

	_, isLive := l.variables[v]
	if isLive {
		delete(l.variables, v)
	}
	return isLive
}

func (l *liveTracker) RemoveRuleIfLive(r Rule) bool {
	l.Lock()
	defer l.Unlock()

	_, isLive := l.rules[r]
	if isLive {
		delete(l.rules, r)
	}
	return isLive
}

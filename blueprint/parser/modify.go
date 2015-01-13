package parser

func AddStringToList(value *Value, s string) (modified bool) {
	if value.Type != List {
		panic("expected list value, got " + value.Type.String())
	}

	for _, v := range value.ListValue {
		if v.Type != String {
			panic("expected string in list, got " + value.Type.String())
		}

		if v.StringValue == s {
			// string already exists
			return false
		}

	}

	value.ListValue = append(value.ListValue, Value{
		Type:        String,
		Pos:         value.EndPos,
		StringValue: s,
	})

	return true
}

func RemoveStringFromList(value *Value, s string) (modified bool) {
	if value.Type != List {
		panic("expected list value, got " + value.Type.String())
	}

	for i, v := range value.ListValue {
		if v.Type != String {
			panic("expected string in list, got " + value.Type.String())
		}

		if v.StringValue == s {
			value.ListValue = append(value.ListValue[:i], value.ListValue[i+1:]...)
			return true
		}

	}

	return false
}

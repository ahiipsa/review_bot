package compare

import (
	"fmt"
	"reflect"
	"time"
)

type CompareResult struct {
	FieldName string
	Value1    interface{}
	Value2    interface{}
}

func Compare(struct1 interface{}, struct2 interface{}) (areEqual bool, differences []*CompareResult, err error) {

	if struct1 == nil || struct2 == nil {
		return false, nil, fmt.Errorf("One of the inputs cannot be nil. struct1: %v, struct2 : %v ", struct1, struct2)
	}

	//Get values of the structs
	v1, v2 := reflect.ValueOf(struct1), reflect.ValueOf(struct2)

	//Handle pointers, if a non-pointer struct is passed in, Indirect will just return the element
	v1, v2 = reflect.Indirect(v1), reflect.Indirect(v2)
	if !v1.IsValid() || !v2.IsValid() {
		return false, nil, fmt.Errorf("Types cannot be nil. v1 %v - v2 %v", v1.IsValid(), v2.IsValid())
	}

	//Cache v1 struct type
	structType := v1.Type()

	//Verify both v1 and v2 are the same type
	if structType != v2.Type() {
		return false, nil, fmt.Errorf("Structs must be the same type. Struct1 %v - Stuct2 -%v", structType, v2.Type())
	}

	//Verify v1 is a struct, if v1 is a struct then v2 is also a struct because we have already verified the types are equal
	if v1.Kind() != reflect.Struct {
		return false, nil, fmt.Errorf("Types must both be structs.  Kind1: %v, Kind2 :v", v1.Kind(), v2.Kind())
	}

	//Initialize differences to ensure length of 0 on return
	differences = make([]*CompareResult, 0)

	for i, numFields := 0, v1.NumField(); i < numFields; i++ {
		//Get values of the structure's fields
		field1, field2 := v1.Field(i), v2.Field(i)

		//Get a reference to the field type
		fieldType := structType.Field(i)

		//If the field name is unexported, skip
		if fieldType.PkgPath != "" {
			continue
		}

		//Handle nil pointers, if a non-pointer field is passed in, Indirect will just return the element
		field1, field2 = reflect.Indirect(field1), reflect.Indirect(field2)

		switch valid1, valid2 := field1.IsValid(), field2.IsValid(); {
		//If both are valid, do nothing
		case valid1 && valid2:
		//If only field1 is valid, set field2 to reflect.Zero
		case valid1:
			field2 = reflect.Zero(field1.Type())
		//If only field1 is valid, set field2 to reflect.Zero
		case valid2:
			field1 = reflect.Zero(field2.Type())
		//Both are invalid so skip loop body
		default:
			continue
		}

		if field1.Kind() == reflect.Interface {
			return false, nil, fmt.Errorf("Type of field cannot be interface. field1: %v, field2: %v", field1, field2)
		}




		val1, val2 := field1.Interface(), field2.Interface();

		switch val1.(type) {
		case int, bool, string, float64, time.Time:
			if val1 != val2 {
				result := &CompareResult{FieldName: fieldType.Name, Value1: val1, Value2: val2}
				differences = append(differences, result)
			}
		default:
			return false, nil, fmt.Errorf("Unsupported type: %v %v", field1.Type(), val1)
		}

	}

	return len(differences) == 0, differences, nil
}
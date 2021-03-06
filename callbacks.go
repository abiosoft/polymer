/*
Copyright 2015 Palm Stone Games, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package polymer

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/gopherjs/gopherjs/js"
)

var typeOfPtrProto = reflect.TypeOf(&Proto{})

func createdCallback(refType reflect.Type) *js.Object {
	return js.MakeFunc(func(this *js.Object, arguments []*js.Object) interface{} {
		// Create a new Go side object
		refVal := reflect.New(refType.Elem())
		proto := refVal.Interface().(Interface)
		refVal = refVal.Elem()

		// Set the proto value, this is needed because we get our callers to embed *polymer.Proto, and it needs to get instantiated
		refVal.FieldByName("Proto").Set(reflect.ValueOf(&Proto{}))

		// Store ourselves in js land so we can map js to proto
		jsMap = append(jsMap, proto)
		this.Set(protoIndexKey, len(jsMap)-1)

		// Set data on the proto
		data := proto.data()
		data.this = this
		data.Element = WrapJSElement(this)

		// Call the proto side callback for user hooks
		proto.Created()

		return nil
	})
}

func readyCallback() *js.Object {
	return js.MakeFunc(func(this *js.Object, arguments []*js.Object) interface{} {
		// Lookup the proto
		proto := jsMap[this.Get(protoIndexKey).Int()]
		refVal := reflect.ValueOf(proto).Elem()
		refType := reflect.TypeOf(proto).Elem()

		// Set initial field values
		for i := 0; i < refType.NumField(); i++ {
			// Get field info first
			fieldVal := refVal.Field(i)
			fieldType := refType.Field(i)

			if fieldType.Anonymous && fieldType.Type == typeOfPtrProto {
				continue
			}

			// If the value in JS is set, we take it over
			// Otherwise, we take over the (usually zeroed) go value and set it in JS
			// We can get away with doing this for only first level values, as they'll either get decoded recursively if they were set
			// Or they'll get set from Go in their entirety if they were undefined
			jsVal := this.Get(fieldType.Name)
			if jsVal == nil || jsVal == js.Undefined {
				this.Set(fieldType.Name, fieldVal.Interface())
			} else {
				currVal := reflect.New(fieldType.Type)
				if err := Decode(jsVal, currVal.Interface()); err != nil {
					panic(fmt.Sprintf("Error while decoding polymer field value for %v: %v", fieldType.Name, err))
				}

				fieldVal.Set(currVal.Elem())
			}
		}

		// Call user hook
		proto.Ready()

		return nil
	})
}

func attachedCallback() *js.Object {
	return js.MakeFunc(func(this *js.Object, arguments []*js.Object) interface{} {
		// Lookup the proto
		proto := jsMap[this.Get(protoIndexKey).Int()]

		// Call the proto side callback for user hooks
		proto.Attached()

		return nil
	})
}

func detachedCallback() *js.Object {
	return js.MakeFunc(func(this *js.Object, arguments []*js.Object) interface{} {
		// Lookup the proto
		proto := jsMap[this.Get(protoIndexKey).Int()]

		// Call the proto side callback for user hooks
		proto.Detached()

		return nil
	})
}

func observeShallowCallback(path []string) *js.Object {
	return js.MakeFunc(func(this *js.Object, jsArgs []*js.Object) interface{} {
		setObservedValue(jsMap[this.Get(protoIndexKey).Int()], path, jsArgs[0])
		return nil
	})
}

func observeDeepCallback() *js.Object {
	return js.MakeFunc(func(this *js.Object, jsArgs []*js.Object) interface{} {
		record := jsArgs[0]
		setObservedValue(jsMap[this.Get(protoIndexKey).Int()], strings.Split(record.Get("path").String(), "."), record.Get("value"))
		return nil
	})
}

func setObservedValue(proto Interface, path []string, val *js.Object) {
	refVal := reflect.ValueOf(proto).Elem()
	for _, curr := range path {
		if curr[0] == '#' {
			index, err := strconv.ParseInt(curr[1:], 10, 32)
			if err != nil {
				panic(err)
			}

			refVal = refVal.Index(int(index))
		} else {
			refVal = refVal.FieldByName(curr)
		}

		if refVal.Kind() == reflect.Ptr {
			refVal = refVal.Elem()
		}
	}

	decodeRaw(val, refVal)
}

// reflectArgs builds up reflect args
// We loop through the function arguments and use the types of each argument to decode the jsArgs
// If the function has more arguments than we have jsArgs, they're passed in as Zero values
// If the function has less arguments than jsArgs, the superfluous jsArgs are silently discarded
func reflectArgs(handler reflect.Value, proto Interface, jsArgs []*js.Object) []reflect.Value {
	handlerType := handler.Type()
	reflectArgs := make([]reflect.Value, handlerType.NumIn())

	jsIndex := 0
	for goIndex := 0; goIndex < handlerType.NumIn(); goIndex++ {
		argType := handlerType.In(goIndex)
		if goIndex == 0 && len(reflectArgs) != 0 && argType == reflect.TypeOf(proto) {
			reflectArgs[goIndex] = reflect.ValueOf(proto)
		} else {
			argPtrVal := reflect.New(argType)
			if len(jsArgs) > jsIndex {
				if err := decodeRaw(jsArgs[jsIndex], argPtrVal.Elem()); err != nil {
					panic(fmt.Sprintf("Error while decoding argument %v: %v", goIndex, err))
				}
			}

			reflectArgs[goIndex] = argPtrVal.Elem()
			jsIndex++
		}

	}

	return reflectArgs
}

func eventHandlerCallback(handler reflect.Value) *js.Object {
	return js.MakeFunc(func(this *js.Object, jsArgs []*js.Object) interface{} {
		handler.Call(reflectArgs(handler, jsMap[this.Get(protoIndexKey).Int()], jsArgs))
		return nil
	})
}

func eventChanCallback(handlerChan reflect.Value) *js.Object {
	chanArgType := handlerChan.Type().Elem()
	return js.MakeFunc(func(this *js.Object, jsArgs []*js.Object) interface{} {
		chanArg := reflect.New(chanArgType)
		decodeRaw(jsArgs[0], chanArg.Elem())
		handlerChan.Send(chanArg.Elem())
		return nil
	})
}

func computeCallback(handler reflect.Value) *js.Object {
	return js.MakeFunc(func(this *js.Object, jsArgs []*js.Object) interface{} {
		returnArgs := handler.Call(reflectArgs(handler, jsMap[this.Get(protoIndexKey).Int()], jsArgs))
		return returnArgs[0].Interface()
	})
}

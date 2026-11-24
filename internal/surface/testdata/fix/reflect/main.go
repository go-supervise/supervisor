// Fixture of the source half: a call the analysis cannot resolve.
package main

import "reflect"

func main() {
	reflect.ValueOf(func() {}).Call(nil)
}

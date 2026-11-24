// Fixture of the source half: unsafe in a package of the module.
package main

import "unsafe"

func main() {
	var x int
	_ = unsafe.Pointer(&x)
}

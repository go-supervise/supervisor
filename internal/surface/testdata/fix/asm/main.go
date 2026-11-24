// Fixture of the source half: a function declared without a body, whose
// implementation is an assembly file of the module.
package main

func raw(nr uintptr) uintptr

func main() {
	_ = raw(39)
}

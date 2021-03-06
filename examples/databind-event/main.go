package main

import (
	"fmt"
	"math/rand"

	"code.palmstonegames.com/polymer"
)

func init() {
	polymer.Register(&NameTag{})
}

type NameTag struct {
	*polymer.Proto

	ID   int64  `polymer:"bind"`
	Name string `polymer:"bind"`
}

func (n *NameTag) TagName() string {
	return "name-tag"
}

func (n *NameTag) Created() {
	n.ID = rand.Int63()
}

func (n *NameTag) Ready() {
	fmt.Printf("%v: Initial Name = %v\n", n.ID, n.Name)
}

func (n *NameTag) HandleNameChange(e *polymer.Event) {
	// We need to use async here because we have no guarantee whether the value change or our event gets triggered first
	// to guarantee the same behaviour across browsers, we delay this by 1ms every time using Async
	n.Async(1, func() {
		fmt.Printf("%v: HandleNameChange event. Name = %v\n", n.ID, n.Name)
	})
}

func main() {}

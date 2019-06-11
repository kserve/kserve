package tfmodel

type Node interface {
	Accept(w Visitor)
}
